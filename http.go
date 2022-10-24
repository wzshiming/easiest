package easiest

import (
	"bufio"
	"bytes"
	"io"
	"sync"
)

// getHTTPHeader returns the value of the first header with the given name.
func modifyOrAddHTTPHeader(r io.Reader, key []byte, val func(value []byte) []byte) ([]byte, error) {
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
			buf.Write(key)
			buf.Write([]byte(": "))
			buf.Write(val(nil))
			buf.Write([]byte("\n\n"))
			return buf.Bytes(), nil
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
		got := bytes.TrimSpace(line[len(key)+1:])
		want := val(got)
		if !bytes.Equal(got, want) {
			buf.Truncate(buf.Len() - (len(line) - len(key)))
			buf.Write([]byte(": "))
			buf.Write(want)
			buf.Write([]byte("\n"))
		}
		return buf.Bytes(), nil
	}
}

// readerPool is a pool of bufio.Reader.
var readerPool = &sync.Pool{
	New: func() interface{} {
		return bufio.NewReader(nil)
	},
}
