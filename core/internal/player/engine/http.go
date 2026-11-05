package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Variables so tests can shorten them.
var (
	headerTimeout = 10 * time.Second
	// readStall ends a response that sends nothing for this long.
	readStall = 20 * time.Second
)

// maxStream caps how much one stream may send.
const maxStream = 2 << 30

var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: headerTimeout}).DialContext,
		TLSHandshakeTimeout:   headerTimeout,
		ResponseHeaderTimeout: headerTimeout,
		Proxy:                 http.ProxyFromEnvironment,
	},
	// A stream URL carries credentials; they must not reach another host.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Host != via[0].URL.Host {
			return errors.New("redirected to another host")
		}
		return nil
	},
}

// httpFile reads a stream over http(s). With ranges it seeks with Range
// requests; without, it only reads on from where it is.
type httpFile struct {
	ctx    context.Context
	url    string
	ranges bool
	size   int64 // -1: unknown
	pos    int64

	body    io.ReadCloser
	bodyPos int64
	cancel  context.CancelFunc
	stall   *time.Timer
}

func openHTTP(ctx context.Context, rawURL string) (*httpFile, error) {
	f := &httpFile{ctx: ctx, url: rawURL, size: -1}
	if err := f.request(0); err != nil {
		return nil, err
	}
	return f, nil
}

// request starts a response at off.
func (f *httpFile) request(off int64) error {
	f.drop()
	ctx, cancel := context.WithCancelCause(f.ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		cancel(nil)
		return errors.New("the stream address is not a URL")
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", off))
	stall := time.AfterFunc(headerTimeout+readStall, func() { cancel(errors.New("the server stopped sending")) })
	resp, err := httpClient.Do(req)
	if err != nil {
		stall.Stop()
		cancel(nil)
		// The URL carries credentials; keep it out of the error.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		f.ranges = true
		if i := strings.LastIndexByte(resp.Header.Get("Content-Range"), '/'); i >= 0 {
			if n, err := strconv.ParseInt(resp.Header.Get("Content-Range")[i+1:], 10, 64); err == nil {
				f.size = n
			}
		}
	case http.StatusOK:
		if off > 0 {
			resp.Body.Close()
			stall.Stop()
			cancel(nil)
			return errNotSeekable
		}
		f.ranges = resp.Header.Get("Accept-Ranges") == "bytes" && resp.ContentLength > 0
		f.size = resp.ContentLength
	default:
		resp.Body.Close()
		stall.Stop()
		cancel(nil)
		return fmt.Errorf("server answered %s", resp.Status)
	}
	f.body, f.bodyPos, f.stall = resp.Body, off, stall
	f.cancel = func() { cancel(nil) }
	stall.Reset(readStall)
	return nil
}

func (f *httpFile) drop() {
	if f.body == nil {
		return
	}
	f.stall.Stop()
	f.cancel()
	f.body.Close()
	f.body = nil
}

func (f *httpFile) Read(p []byte) (int, error) {
	if f.pos >= maxStream || (f.size >= 0 && f.pos >= f.size) {
		return 0, io.EOF
	}
	if f.body == nil || f.pos < f.bodyPos || (f.ranges && f.pos > f.bodyPos+256<<10) {
		if !f.ranges && f.pos != 0 {
			return 0, errNotSeekable
		}
		if err := f.request(f.pos); err != nil {
			return 0, err
		}
	}
	// Short forward seeks read on.
	for f.bodyPos < f.pos {
		n, err := io.CopyN(io.Discard, f.body, f.pos-f.bodyPos)
		f.bodyPos += n
		if err != nil {
			return 0, err
		}
	}
	n, err := f.body.Read(p[:min(int64(len(p)), maxStream-f.pos)])
	f.pos += int64(n)
	f.bodyPos = f.pos
	if n > 0 {
		f.stall.Reset(readStall)
	}
	return n, err
}

func (f *httpFile) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		off += f.pos
	case io.SeekEnd:
		if f.size < 0 {
			return 0, errNotSeekable
		}
		off += f.size
	}
	if off < 0 {
		return 0, errors.New("seek before the start")
	}
	if !f.ranges && off < f.bodyPos {
		return 0, errNotSeekable
	}
	f.pos = off
	return off, nil
}

func (f *httpFile) Close() error {
	f.drop()
	return nil
}
