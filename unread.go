package easiest

import (
	"bytes"
	"io"
	"net"

	"github.com/wzshiming/sni"
)

// tlsHostWithConn returns the TLS host
func tlsHostWithConn(conn net.Conn) (net.Conn, string, error) {
	buf := bytes.NewBuffer(nil)
	host, err := sni.TLSHost(io.TeeReader(conn, buf))
	if err != nil {
		return nil, "", err
	}
	return wrapUnreadConn(conn, buf.Bytes()), host, nil
}

// httpPathWithConn returns the path
func httpPathWithConn(conn net.Conn) (net.Conn, string, error) {
	buf := bytes.NewBuffer(nil)
	path, err := getHTTPPath(io.TeeReader(conn, buf))
	if err != nil {
		return nil, "", err
	}
	return wrapUnreadConn(conn, buf.Bytes()), path, nil
}

func wrapUnreadConn(conn net.Conn, prefix []byte) net.Conn {
	if len(prefix) == 0 {
		return conn
	}
	if us, ok := conn.(*unreadConn); ok {
		us.Reader = wrapUnread(us.Reader, prefix)
		return us
	}
	return &unreadConn{
		Reader: wrapUnread(conn, prefix),
		Conn:   conn,
	}
}

type unreadConn struct {
	io.Reader
	net.Conn
}

func (c *unreadConn) Read(p []byte) (n int, err error) {
	return c.Reader.Read(p)
}

func wrapUnread(reader io.Reader, prefix []byte) io.Reader {
	if len(prefix) == 0 {
		return reader
	}
	if ur, ok := reader.(*unread); ok {
		ur.prefix = append(prefix, ur.prefix...)
		return reader
	}
	return &unread{
		prefix: prefix,
		reader: reader,
	}
}

type unread struct {
	prefix []byte
	reader io.Reader
}

func (u *unread) Read(p []byte) (n int, err error) {
	if len(u.prefix) == 0 {
		return u.reader.Read(p)
	}
	n = copy(p, u.prefix)
	if n <= len(u.prefix) {
		u.prefix = u.prefix[n:]
		if len(u.prefix) == 0 {
			u.prefix = nil
		}
		return n, nil
	}
	a, err := u.reader.Read(p[n:])
	if err == io.EOF {
		err = nil
	}
	n += a
	return n, err
}
