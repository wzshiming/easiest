package easiest

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
)

func connHTTPRedirect(conn net.Conn, url string, code int) error {
	var data = "HTTP/1.1 " + strconv.FormatInt(int64(code), 10) + " " + http.StatusText(code) + "\r\n" +
		"Location: " + url + "\r\n" +
		"Connection: close\r\n" +
		"\r\n"

	_, err := conn.Write([]byte(data))
	return err
}

func connAddHTTPHeader(conn net.Conn, key, val []byte) (net.Conn, error) {
	header, err := addHTTPHeader(conn, key, val)
	if err != nil {
		return nil, err
	}
	return wrapUnreadConn(conn, header), nil
}

func addHTTPHeader(r io.Reader, key []byte, val []byte) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	r = io.TeeReader(r, buf)
	reader := readerPool.Get().(*bufio.Reader)
	reader.Reset(r)
	defer func() {
		reader.Reset(nil)
		readerPool.Put(reader)
	}()

	_, _, err := reader.ReadLine() // skip the first line
	if err != nil {
		return nil, err
	}
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		// check for the end of the headers
		if len(line) == 0 {
			data := bytes.TrimSuffix(buf.Bytes(), []byte("\r\n"))
			data = append(data, key...)
			data = append(data, []byte(": ")...)
			data = append(data, val...)
			data = append(data, []byte("\r\n\r\n")...)
			return data, nil
		}
	}
}

// readerPool is a pool of bufio.Reader.
var readerPool = &sync.Pool{
	New: func() interface{} {
		return bufio.NewReader(nil)
	},
}

// getHTTPHeader returns the value of the first header with the given name.
func getHTTPHeader(r io.Reader, key []byte) (string, error) {
	reader := readerPool.Get().(*bufio.Reader)
	reader.Reset(r)
	defer func() {
		reader.Reset(nil)
		readerPool.Put(reader)
	}()

	_, _, err := reader.ReadLine() // skip the first line
	if err != nil {
		return "", err
	}
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			return "", err
		}
		// check for the end of the headers
		if len(line) == 0 {
			return "", nil
		}

		// check for the key
		if len(line) <= len(key) {
			continue
		}
		if line[len(key)] != ':' {
			continue
		}
		if !bytes.Equal(bytes.ToLower(line[:len(key)]), key) {
			continue
		}
		return string(bytes.TrimSpace(line[len(key)+1:])), nil
	}
}

// getHTTPPath returns the value of the first header with the given name.
func getHTTPPath(r io.Reader) (string, error) {
	reader := readerPool.Get().(*bufio.Reader)
	reader.Reset(r)
	defer func() {
		reader.Reset(nil)
		readerPool.Put(reader)
	}()

	command, _, err := reader.ReadLine() // skip the first line
	if err != nil {
		return "", err
	}

	begin := bytes.IndexByte(command, ' ')
	end := bytes.IndexByte(command[begin+1:], ' ')
	return string(command[begin+1 : begin+1+end]), nil
}
