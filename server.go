package easiest

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

const (
	httpPort  = ":80"
	httpsPort = ":443"
)

type Server struct {
	route     map[string]string
	tlsConfig *tls.Config
	logger    Logger
}

type Logger interface {
	Println(v ...interface{})
}

func NewServer(route map[string]string, tlsDir string, logger Logger) *Server {
	return &Server{
		route:     route,
		tlsConfig: newAcme(nil, tlsDir),
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
			err := s.handleHTTP(ctx, conn)
			if err != nil {
				if s.logger != nil && !isClosedConnError(err) && err != io.EOF {
					s.logger.Println("handleHTTP", err)
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

	target := s.route[host]
	if target == "" {
		return fmt.Errorf("not route %q", host)
	}

	remote, err := net.Dial("tcp", target)
	if err != nil {
		return err
	}

	injectedConn, err := s.injectForwardedFor(conn, conn.RemoteAddr().String())
	if err != nil {
		return err
	}
	return s.tunnel(ctx, injectedConn, remote)
}

func (s *Server) startTLS(ctx context.Context, svc net.Listener) error {
	for {
		conn, err := svc.Accept()
		if err != nil {
			return err
		}
		go func() {
			err := s.handleTLS(ctx, conn)
			if err != nil {
				if s.logger != nil && !isClosedConnError(err) && err != io.EOF {
					s.logger.Println("handleTLS", err)
				}
			}
		}()
	}
}

func (s *Server) handleTLS(ctx context.Context, conn net.Conn) error {
	conn, host, err := tlsHostWithConn(conn)
	if err != nil {
		return err
	}

	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}

	target := s.route[host]
	if target == "" {
		return fmt.Errorf("not route %q", host)
	}

	tlsConn := tls.Server(conn, s.tlsConfig)
	err = tlsConn.HandshakeContext(ctx)
	if err != nil {
		return err
	}

	remote, err := net.Dial("tcp", target)
	if err != nil {
		return err
	}

	injectedConn, err := s.injectForwardedFor(tlsConn, conn.RemoteAddr().String())
	if err != nil {
		return err
	}
	return s.tunnel(ctx, injectedConn, remote)
}

func (s *Server) injectForwardedFor(conn net.Conn, remoteAddr string) (net.Conn, error) {
	header, err := modifyOrAddHTTPHeader(conn, []byte("x-forwarded-for"), func(prior []byte) []byte {
		if len(prior) > 0 {
			return []byte(string(prior) + ", " + remoteAddr)
		} else {
			return []byte(remoteAddr)
		}
	})
	if err != nil {
		return nil, err
	}
	return wrapUnreadConn(conn, header), nil
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
