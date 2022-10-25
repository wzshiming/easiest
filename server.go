package easiest

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	httpPort  = ":80"
	httpsPort = ":443"
)

type Server struct {
	route     map[string]Route
	tlsConfig *tls.Config
	logger    Logger
}

type Logger interface {
	Println(v ...interface{})
}

func NewServer(conf Config, logger Logger) *Server {
	route := map[string]Route{}
	for _, r := range conf.Routes {
		route[r.Domain] = r
	}
	return &Server{
		route:     route,
		tlsConfig: newAcme(nil, conf.TlsDir),
		logger:    logger,
	}
}

func (s *Server) Run(ctx context.Context) error {
	httpsServer, err := net.Listen("tcp", httpsPort)
	if err != nil {
		return err
	}
	httpServer, err := net.Listen("tcp", httpPort)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := s.startHTTP(ctx, httpServer)
		if err != nil {
			if s.logger != nil {
				s.logger.Println("startHTTP", err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		err := s.startTLS(ctx, httpsServer)
		if err != nil {
			if s.logger != nil {
				s.logger.Println("startTLS", err)
			}
		}
	}()

	wg.Wait()
	return nil
}

func (s *Server) startHTTP(ctx context.Context, svc net.Listener) error {
	for {
		conn, err := svc.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer conn.Close()
			err := s.handleHTTP(ctx, conn)
			if err != nil {
				if s.logger != nil && !isClosedConnError(err) && err != io.EOF {
					s.logger.Println("handleHTTP", err)
				}
			}
		}()
	}
}

func (s *Server) startTLS(ctx context.Context, svc net.Listener) error {
	for {
		conn, err := svc.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer conn.Close()
			err := s.handleTLS(ctx, conn)
			if err != nil {
				if s.logger != nil && !isClosedConnError(err) && err != io.EOF {
					s.logger.Println("handleTLS", err)
				}
			}
		}()
	}
}

func (s *Server) handleHTTP(ctx context.Context, conn net.Conn) error {
	conn, host, err := httpHostWithConn(conn)
	if err != nil {
		return err
	}

	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}

	route, ok := s.route[host]
	if !ok {
		return fmt.Errorf("not route %q", host)
	}

	if !route.HTTP.ForceTLS {
		return s.bind(ctx, route, conn)
	}

	conn, path, err := httpPathWithConn(conn)
	if err != nil {
		return err
	}
	u, err := url.Parse(path)
	if err != nil {
		return err
	}
	u.Host = host
	u.Scheme = "https"
	u.Fragment = ""
	return connHTTPRedirect(conn, u.String(), http.StatusFound)
}

func (s *Server) handleTLS(ctx context.Context, conn net.Conn) error {
	conn, host, err := tlsHostWithConn(conn)
	if err != nil {
		return err
	}

	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}

	route, ok := s.route[host]
	if !ok {
		return fmt.Errorf("not route %q", host)
	}

	tlsConn := tls.Server(conn, s.tlsConfig)
	err = tlsConn.HandshakeContext(ctx)
	if err != nil {
		return err
	}

	return s.bind(ctx, route, tlsConn)
}

func (s *Server) bind(ctx context.Context, route Route, downstream net.Conn) error {
	upstream, err := dialTarget(route.Target)
	if err != nil {
		return err
	}

	if route.HTTP.HeaderForwardedFor {
		downstream, err = connAddHTTPHeader(downstream, []byte("X-Forwarded-For"), []byte(downstream.RemoteAddr().String()))
		if err != nil {
			return err
		}
	}

	if len(route.Replaces) != 0 {
		var reuse func()
		downstream, upstream, reuse = s.repliace(route, downstream, upstream)
		if reuse != nil {
			defer reuse()
		}
	}
	return s.tunnel(ctx, downstream, upstream)
}

func (s *Server) repliace(route Route, newConn, oldConn net.Conn) (net.Conn, net.Conn, func()) {
	if len(route.Replaces) == 0 {
		return newConn, oldConn, nil
	}
	bufs := make([]interface{}, 0, len(route.Replaces))
	for _, replace := range route.Replaces {
		new := []byte(replace.New)
		old := []byte(replace.Old)
		buf1 := bytesPool.Get().([]byte)
		buf2 := bytesPool.Get().([]byte)
		newConn = connReplaceReader(newConn, new, old, buf1)
		oldConn = connReplaceReader(oldConn, old, new, buf2)
		bufs = append(bufs, buf1, buf2)
	}
	return newConn, oldConn, func() {
		for _, buf := range bufs {
			bytesPool.Put(buf)
		}
	}
}

func (s *Server) tunnel(ctx context.Context, c1, c2 io.ReadWriteCloser) error {
	buf1 := bytesPool.Get().([]byte)
	buf2 := bytesPool.Get().([]byte)
	defer func() {
		bytesPool.Put(buf1)
		bytesPool.Put(buf2)
	}()
	return tunnel(ctx, c1, c2, buf1, buf2)
}

// readerPool is a pool of bufio.Reader.
var bytesPool = &sync.Pool{
	New: func() interface{} {
		return make([]byte, 32*1024)
	},
}
