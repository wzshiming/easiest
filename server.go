package easiest

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
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
	conn, host, err := connGetHTTPHost(conn)
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
	upstream, host, err := dialTarget(route.Target)
	if err != nil {
		return err
	}
	defer upstream.Close()

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	br := readerPool.Get().(*bufio.Reader)
	br.Reset(downstream)
	defer readerPool.Put(br)
	err = req.Read(br)
	if err != nil {
		return err
	}
	req.SetHost(host)
	req.SetConnectionClose()

	if route.HTTP.HeaderForwardedFor {
		req.Header.Add("X-Forwarded-For", downstream.RemoteAddr().String())
	}

	if len(route.Replaces) != 0 &&
		req.Header.ContentLength() != 0 &&
		(bytes.HasPrefix(req.Header.ContentType(), []byte("text/")) ||
			bytes.HasPrefix(req.Header.ContentType(), []byte("application/javascript"))) {
		body, err := req.BodyUncompressed()
		if err != nil {
			return err
		}

		for _, replace := range route.Replaces {
			body = bytes.Replace(body, []byte(replace.New), []byte(replace.Old), -1)
		}

		req.SetBodyRaw(body)
		req.Header.Set("Content-Encoding", "")
	}

	_, err = req.WriteTo(upstream)
	if err != nil {
		return err
	}

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	br.Reset(upstream)
	err = resp.Read(br)
	if err != nil {
		return err
	}

	if len(route.Replaces) != 0 &&
		resp.Header.ContentLength() != 0 &&
		(bytes.HasPrefix(resp.Header.ContentType(), []byte("text/")) ||
			bytes.HasPrefix(resp.Header.ContentType(), []byte("application/javascript"))) {
		body, err := resp.BodyUncompressed()
		if err != nil {
			return err
		}

		for _, replace := range route.Replaces {
			body = bytes.Replace(body, []byte(replace.Old), []byte(replace.New), -1)
		}

		resp.SetBodyRaw(body)
		resp.Header.Set("Content-Encoding", "")
	}

	resp.SetConnectionClose()
	_, err = resp.WriteTo(downstream)
	if err != nil {
		return err
	}
	return nil
}

// readerPool is a pool of bufio.Reader.
var bytesPool = &sync.Pool{
	New: func() interface{} {
		return make([]byte, 32*1024)
	},
}
