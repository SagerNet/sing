package http

import (
	std_bufio "bufio"
	"io"
	"net/http"

	E "github.com/sagernet/sing/common/exceptions"
)

var errHeaderTooLarge = E.New("http: request header too large")

type Reader struct {
	*std_bufio.Reader
	limiter *readLimiter
}

func NewReader(upstream io.Reader) *Reader {
	limiter := &readLimiter{reader: upstream, remaining: -1}
	return &Reader{
		Reader:  std_bufio.NewReader(limiter),
		limiter: limiter,
	}
}

func (r *Reader) ReadRequest() (*http.Request, error) {
	r.limiter.remaining = http.DefaultMaxHeaderBytes
	request, err := ReadRequest(r.Reader)
	r.limiter.remaining = -1
	return request, err
}

type readLimiter struct {
	reader    io.Reader
	remaining int64
}

func (r *readLimiter) Read(p []byte) (int, error) {
	if r.remaining < 0 {
		return r.reader.Read(p)
	}
	if r.remaining == 0 {
		return 0, errHeaderTooLarge
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}
