package easiest

import (
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
	"github.com/valyala/fasthttp/fasthttpproxy"
)

const (
	httpPort  = ":80"
	httpsPort = ":443"
)

type Server struct {
	route      map[string]Route
	tlsConfig  *tls.Config
	logger     Logger
	httpServer fasthttp.Server
	httpClient fasthttp.Client
}

type Logger interface {
	Println(v ...interface{})
}

func NewServer(conf Config, logger Logger) *Server {
	route := map[string]Route{}
	for _, r := range conf.Routes {
		route[r.Domain] = r
	}
	s := &Server{
		route:     route,
		tlsConfig: newAcme(nil, conf.TlsDir),
		logger:    logger,
	}
	s.httpClient.Dial = fasthttpproxy.FasthttpProxyHTTPDialer()
	s.httpServer.Handler = s.handler
	return s
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

func (s *Server) dialTarget(o string) (net.Conn, string, error) {
	u, err := url.Parse(o)
	if err != nil {
		return nil, "", err
	}
	switch u.Scheme {
	case "http":
		port := u.Port()
		if port == "" {
			port = "80"
		}
		host := u.Hostname()

		conn, err := s.httpClient.Dial(net.JoinHostPort(host, port))
		if err != nil {
			return nil, "", err
		}
		return conn, host, nil
	case "https":
		port := u.Port()
		if port == "" {
			port = "443"
		}
		host := u.Hostname()
		conn, err := s.httpClient.Dial(net.JoinHostPort(host, port))
		if err != nil {
			return nil, "", err
		}

		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, "", err
		}
		return tlsConn, host, nil
	}
	return nil, "", fmt.Errorf("unsupported scheme %q", u.Scheme)
}

func (s *Server) bind(ctx context.Context, route Route, downstream net.Conn) error {
	if !route.Stream {
		return s.httpServer.ServeConn(downstream)
	} else {
		upstream, _, err := s.dialTarget(route.Target)
		if err != nil {
			return err
		}
		defer upstream.Close()
		return s.stream(ctx, route, upstream, downstream)
	}
}

func (s *Server) handler(ctx *fasthttp.RequestCtx) {
	err := s.handlerErr(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Println("handlerErr", err)
		}
	}
}

func (s *Server) handlerErr(ctx *fasthttp.RequestCtx) error {
	req := &ctx.Request
	resp := &ctx.Response
	host := string(req.Host())

	route, ok := s.route[host]
	if !ok {
		return fmt.Errorf("not route %q", host)
	}

	u, err := url.Parse(route.Target)
	if err != nil {
		return err
	}

	req.URI().SetScheme(u.Scheme)
	req.SetHost(u.Host)
	req.SetConnectionClose()

	if route.HTTP.HeaderForwardedFor {
		req.Header.Add(fasthttp.HeaderXForwardedFor, ctx.RemoteAddr().String())
	}

	if len(route.Replaces) != 0 {
		length := req.Header.ContentLength()
		if length != 0 &&
			(bytes.HasPrefix(req.Header.ContentType(), []byte("text/")) ||
				bytes.HasPrefix(req.Header.ContentType(), []byte("application/javascript"))) {

			if length < 8*1024*1024 {
				body, err := req.BodyUncompressed()
				if err != nil {
					return err
				}

				for _, replace := range route.Replaces {
					body = bytes.Replace(body, []byte(replace.New), []byte(replace.Old), -1)
				}

				req.SetBodyRaw(body)
				req.Header.Set(fasthttp.HeaderContentEncoding, "")
			}
		}

		referer := req.Header.Referer()
		if len(referer) != 0 {
			for _, replace := range route.Replaces {
				referer = bytes.Replace(referer, []byte(replace.New), []byte(replace.Old), -1)
			}
			req.Header.SetRefererBytes(referer)
		}

		origin := req.Header.Peek(fasthttp.HeaderOrigin)
		if len(origin) != 0 {
			for _, replace := range route.Replaces {
				origin = bytes.Replace(origin, []byte(replace.New), []byte(replace.Old), -1)
			}
			req.Header.SetBytesV(fasthttp.HeaderOrigin, origin)
		}
	}
	req.SetConnectionClose()

	err = s.httpClient.Do(req, resp)
	if err != nil {
		return err
	}

	if len(route.Replaces) != 0 {
		length := resp.Header.ContentLength()
		if length != 0 &&
			(bytes.HasPrefix(resp.Header.ContentType(), []byte("text/")) ||
				bytes.HasPrefix(resp.Header.ContentType(), []byte("application/javascript"))) {

			if length < 8*1024*1024 {
				body, err := resp.BodyUncompressed()
				if err != nil {
					return err
				}

				for _, replace := range route.Replaces {
					body = bytes.Replace(body, []byte(replace.Old), []byte(replace.New), -1)
				}

				resp.SetBodyRaw(body)
				resp.Header.Set(fasthttp.HeaderContentEncoding, "")
			}
		}

		timingAllowOrigin := resp.Header.Peek(fasthttp.HeaderTimingAllowOrigin)
		if len(timingAllowOrigin) != 0 {
			for _, replace := range route.Replaces {
				timingAllowOrigin = bytes.Replace(timingAllowOrigin, []byte(replace.Old), []byte(replace.New), -1)
			}
			resp.Header.SetBytesV(fasthttp.HeaderTimingAllowOrigin, timingAllowOrigin)
		}

		if code := resp.StatusCode(); code >= 300 && code < 400 {
			location := resp.Header.Peek(fasthttp.HeaderLocation)
			if len(location) != 0 {
				for _, replace := range route.Replaces {
					location = bytes.Replace(location, []byte(replace.Old), []byte(replace.New), -1)
				}
				resp.Header.SetBytesV(fasthttp.HeaderLocation, location)
			}
		}
	}

	if len(req.Header.Referer()) != 0 {
		resp.Header.Set(fasthttp.HeaderAccessControlAllowOrigin, "*")
	}

	resp.Header.Del(fasthttp.HeaderContentSecurityPolicy)

	resp.SetConnectionClose()

	return nil
}

func (s *Server) stream(ctx context.Context, route Route, upstream, downstream net.Conn) error {
	if len(route.Replaces) != 0 {
		var reuse func()
		downstream, upstream, reuse = s.replace(route, downstream, upstream)
		if reuse != nil {
			defer reuse()
		}
	}
	return s.tunnel(ctx, downstream, upstream)
}

func (s *Server) replace(route Route, downstream, upstream net.Conn) (net.Conn, net.Conn, func()) {
	if len(route.Replaces) == 0 {
		return downstream, upstream, nil
	}
	bufs := make([]interface{}, 0, len(route.Replaces))
	for _, replace := range route.Replaces {
		new := []byte(replace.New)
		old := []byte(replace.Old)
		buf1 := bytesPool.Get().([]byte)
		buf2 := bytesPool.Get().([]byte)
		downstream = connReplaceReader(downstream, new, old, buf1)
		upstream = connReplaceReader(upstream, old, new, buf2)
		bufs = append(bufs, buf1, buf2)
	}
	return downstream, upstream, func() {
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
