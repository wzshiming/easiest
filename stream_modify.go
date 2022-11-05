package easiest

import (
	"bytes"
	"io"
	"net"
	"time"
)

func connReplaceReader(conn net.Conn, old, new []byte, buf []byte) net.Conn {
	type Conn interface {
		//Read(b []byte) (n int, err error)

		Write(b []byte) (n int, err error)
		Close() error

		LocalAddr() net.Addr

		RemoteAddr() net.Addr
		SetDeadline(t time.Time) error

		SetReadDeadline(t time.Time) error
		SetWriteDeadline(t time.Time) error
	}
	return struct {
		Conn
		io.Reader
	}{
		Conn:   conn,
		Reader: newReplaceReader(conn, old, new, buf),
	}
}

func newReplaceReader(r io.Reader, old, new []byte, buf []byte) io.Reader {
	if buf == nil {
		buf = make([]byte, 0, 32*1024)
	}
	return &replaceReader{
		reader: r,
		old:    old,
		buf:    buf[:0],
		new:    new,
	}
}

type replaceReader struct {
	reader   io.Reader
	buf      []byte
	old, new []byte
}

func (r *replaceReader) Read(p []byte) (int, error) {
	var n int

	for n+len(r.new) < len(p) {
		if len(r.buf) != 0 {
			m := copy(p[n:], r.buf)
			n += m
			copy(r.buf, r.buf[m:])
			r.buf = r.buf[:len(r.buf)-m]
			return n, nil
		}

		m, err := r.reader.Read(r.buf[:cap(r.buf)])
		if err != nil {
			return 0, err
		}
		r.buf = r.buf[:m]

		r.replaceAll()
	}
	return n, nil
}

func (r *replaceReader) replaceAll() {
	off := 0
	for off != -1 {
		off = r.replace(off)
	}
}

func (r *replaceReader) replace(off int) int {
	if len(r.buf) <= off {
		return -1
	}
	i := off
	for !bytes.HasPrefix(r.buf[i:], r.old) {
		l := bytes.IndexByte(r.buf[i+1:], r.old[0])
		if l == -1 {
			return -1
		}
		i = i + l + 1
	}

	if len(r.old) == len(r.new) {
		copy(r.buf[i:], r.new)
	} else {
		copy(r.buf[i+len(r.new):cap(r.buf)], r.buf[i+len(r.old):])
		copy(r.buf[i:], r.new)
		r.buf = r.buf[:len(r.buf)-len(r.old)+len(r.new)]
	}

	return i + len(r.new) + 1
}
