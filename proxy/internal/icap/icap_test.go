package icap_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/icap"
	"github.com/yatuk/tamga/internal/icap/icaptest"
)

// fakeHandler wants requests to example.invalid. A body that contains
// "block" is answered with a 403, one that contains "mask" has "mask"
// replaced, and anything else passes.
type fakeHandler struct {
	mu    sync.Mutex
	limit int
	seen  []*icap.Request
}

func (h *fakeHandler) Inspect(req *icap.Request) (bool, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	limit := h.limit
	if limit == 0 {
		limit = 1 << 20
	}
	return req.HTTP.URL.Host == "example.invalid" || req.HTTP.Host == "example.invalid", limit
}

func (h *fakeHandler) Adapt(_ context.Context, req *icap.Request) icap.Result {
	h.mu.Lock()
	h.seen = append(h.seen, req)
	h.mu.Unlock()
	switch {
	case req.BodyTooLarge:
		return icap.Result{Kind: icap.Respond, Status: http.StatusRequestEntityTooLarge, ResponseBody: []byte("too large")}
	case bytes.Contains(req.Body, []byte("block")):
		return icap.Result{Kind: icap.Respond, Status: http.StatusForbidden,
			ResponseHeader: http.Header{"Content-Type": {"application/json"}}, ResponseBody: []byte(`{"blocked":true}`)}
	case bytes.Contains(req.Body, []byte("mask")):
		return icap.Result{Kind: icap.Modified, Body: bytes.ReplaceAll(req.Body, []byte("mask"), []byte("[X]")), DropHeaders: []string{"X-Drop"}}
	}
	return icap.Result{Kind: icap.Unmodified}
}

func (h *fakeHandler) ISTag() string { return "tag-1 \"x\"" }

func (h *fakeHandler) last() *icap.Request {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.seen) == 0 {
		return nil
	}
	return h.seen[len(h.seen)-1]
}

func start(t *testing.T, h icap.Handler, tune func(*icap.Server)) (string, *icap.Server) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &icap.Server{Handler: h}
	if tune != nil {
		tune(s)
	}
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	})
	return ln.Addr().String(), s
}

func dial(t *testing.T, addr string) *icaptest.Conn {
	t.Helper()
	c, err := icaptest.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

const postHead = "POST https://example.invalid/v1/chat HTTP/1.1\r\nHost: example.invalid\r\nContent-Type: application/json\r\nContent-Length: %d\r\nX-Drop: yes"

func post(body string) string {
	return strings.Replace(postHead, "%d", itoa(len(body)), 1)
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestOptions(t *testing.T) {
	addr, _ := start(t, &fakeHandler{}, nil)
	c := dial(t, addr)
	if err := c.Write("OPTIONS icap://" + addr + "/reqmod ICAP/1.0\r\nHost: " + addr + "\r\nEncapsulated: null-body=0\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	rep, err := c.Read()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 {
		t.Fatalf("status %d", rep.Status)
	}
	for k, want := range map[string]string{"Methods": "REQMOD", "Allow": "204", "Preview": "0", "Istag": `"tag-1x"`, "Encapsulated": "null-body=0"} {
		if got := rep.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	// The connection is kept: a second OPTIONS, without Encapsulated, is
	// answered on it.
	if err := c.Write("OPTIONS icap://" + addr + "/reqmod ICAP/1.0\r\nHost: " + addr + "\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if rep, err := c.Read(); err != nil || rep.Status != 200 {
		t.Fatalf("second OPTIONS: %v %v", rep, err)
	}
}

func TestReqmodNotWantedIs204AfterPreview(t *testing.T) {
	h := &fakeHandler{}
	addr, _ := start(t, h, nil)
	c := dial(t, addr)
	body := []byte(`{"x":"block"}`)
	rep, err := c.Do(icaptest.Request{HTTP: strings.Replace(post(string(body)), "example.invalid", "other.invalid", -1), Body: body, Preview: 0})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 204 {
		t.Fatalf("status %d", rep.Status)
	}
	if h.last() != nil {
		t.Fatal("Adapt was called for a request Inspect did not want")
	}
	// Keep-alive: the next request goes over the same connection.
	rep, err = c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: 0})
	if err != nil || rep.Status != 200 || rep.Response == nil || rep.Response.StatusCode != 403 {
		t.Fatalf("second request: %+v %v", rep, err)
	}
}

func TestReqmodPreviewThen100Continue(t *testing.T) {
	h := &fakeHandler{}
	addr, _ := start(t, h, nil)
	c := dial(t, addr)
	body := []byte(`{"messages":[{"content":"please mask this and mask that"}]}`)
	rep, err := c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: 0, Chunk: 7})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 || rep.Request == nil {
		t.Fatalf("status %d, request %v", rep.Status, rep.Request)
	}
	want := `{"messages":[{"content":"please [X] this and [X] that"}]}`
	if string(rep.Body) != want {
		t.Fatalf("body %q", rep.Body)
	}
	if got := rep.Request.Header.Get("Content-Length"); got != itoa(len(want)) {
		t.Errorf("Content-Length %q", got)
	}
	if rep.Request.Header.Get("X-Drop") != "" {
		t.Error("dropped header still there")
	}
	if rep.Request.Header.Get("Content-Type") != "application/json" || rep.Request.URL.String() != "https://example.invalid/v1/chat" {
		t.Errorf("request head changed: %q", rep.RawHTTP)
	}
	if got := string(h.last().Body); got != string(body) {
		t.Errorf("handler saw %q", got)
	}
}

func TestReqmodPreviewWithIEOF(t *testing.T) {
	h := &fakeHandler{}
	addr, _ := start(t, h, nil)
	c := dial(t, addr)
	body := []byte(`{"q":"block me"}`)
	// The whole body fits in the preview: no 100 Continue, the answer comes
	// at once.
	rep, err := c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 || rep.Response == nil || rep.Response.StatusCode != 403 {
		t.Fatalf("reply %+v", rep)
	}
	if string(rep.Body) != `{"blocked":true}` || rep.Response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("response %q %v", rep.Body, rep.Response.Header)
	}
}

func TestReqmodNoPreviewChunksAndExtensions(t *testing.T) {
	h := &fakeHandler{}
	addr, _ := start(t, h, nil)
	c := dial(t, addr)
	body := `{"a":"clean text"}`
	raw := icaptest.Head(icaptest.Request{HTTP: post(body), Body: []byte(body), Preview: -1, Header: map[string]string{"Allow": "204"}}, addr)
	// Chunks with an extension, in upper-case hex.
	raw += "5;name=value\r\n" + body[:5] + "\r\nD\r\n" + body[5:] + "\r\n0\r\n\r\n"
	if err := c.Write(raw); err != nil {
		t.Fatal(err)
	}
	rep, err := c.Read()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 204 {
		t.Fatalf("status %d", rep.Status)
	}
	if got := string(h.last().Body); got != body {
		t.Fatalf("handler saw %q", got)
	}
}

func TestReqmodEchoWithout204(t *testing.T) {
	h := &fakeHandler{}
	addr, _ := start(t, h, nil)
	c := dial(t, addr)
	body := []byte(`{"a":"clean"}`)
	// Wanted, unchanged, no Allow: 204 and no preview: echoed.
	rep, err := c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: -1, Chunk: 3})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 || string(rep.Body) != string(body) || !strings.HasPrefix(string(rep.RawHTTP), "POST https://example.invalid/v1/chat HTTP/1.1\r\n") {
		t.Fatalf("echo %d %q %q", rep.Status, rep.RawHTTP, rep.Body)
	}
	// Not wanted, same: echoed, the body streamed through.
	other := strings.Replace(post(string(body)), "example.invalid", "other.invalid", -1)
	rep, err = c.Do(icaptest.Request{HTTP: other, Body: body, Preview: -1, Chunk: 3})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 || string(rep.Body) != string(body) || rep.Request.Host != "other.invalid" {
		t.Fatalf("echo %d %q", rep.Status, rep.Body)
	}
	// A request without a body comes back without one.
	rep, err = c.Do(icaptest.Request{HTTP: "GET https://other.invalid/ HTTP/1.1\r\nHost: other.invalid", Preview: -1})
	if err != nil || rep.Status != 200 || rep.HasBody {
		t.Fatalf("GET echo %+v %v", rep, err)
	}
}

func TestReqmodBodyTooLarge(t *testing.T) {
	h := &fakeHandler{limit: 100}
	addr, _ := start(t, h, nil)
	c := dial(t, addr)
	body := bytes.Repeat([]byte("a"), 5000)
	rep, err := c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: 0, Chunk: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 || rep.Response == nil || rep.Response.StatusCode != 413 {
		t.Fatalf("reply %+v", rep)
	}
	if !h.last().BodyTooLarge || h.last().Body != nil {
		t.Fatal("handler was not told the body was too large")
	}
	// The rest was drained: the connection still works.
	small := []byte(`{"q":"block"}`)
	rep, err = c.Do(icaptest.Request{HTTP: post(string(small)), Body: small, Preview: 0})
	if err != nil || rep.Response == nil || rep.Response.StatusCode != 403 {
		t.Fatalf("after a large body: %+v %v", rep, err)
	}
}

func TestReqmodBodyBeyondDrainClosesConnection(t *testing.T) {
	h := &fakeHandler{limit: 100}
	addr, _ := start(t, h, func(s *icap.Server) { s.DrainBytes = 1000 })
	c := dial(t, addr)
	body := bytes.Repeat([]byte("a"), 50000)
	rep, err := c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: -1, Chunk: 500})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 || rep.Response.StatusCode != 413 || rep.Header.Get("Connection") != "close" {
		t.Fatalf("reply %d %v", rep.Status, rep.Header)
	}
}

func TestMalformedMessages(t *testing.T) {
	cases := []struct {
		name   string
		raw    func(addr string) string
		status int
	}{
		{"garbage", func(string) string { return "hello\r\n\r\n" }, 400},
		{"version", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/2.0\r\nHost: x\r\nEncapsulated: null-body=0\r\n\r\n"
		}, 505},
		{"method", func(a string) string {
			return "BREW icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: null-body=0\r\n\r\n"
		}, 501},
		{"no host", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nEncapsulated: null-body=0\r\n\r\n"
		}, 400},
		{"no encapsulated", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\n\r\n"
		}, 400},
		{"offsets out of order", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: req-hdr=0, req-body=0, null-body=5\r\n\r\n"
		}, 400},
		{"body not last", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: req-body=0, req-hdr=10\r\n\r\n"
		}, 400},
		{"short headers", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: req-hdr=0, null-body=20\r\n\r\nGET / HTTP/1.1\r\nHo\r\n"
		}, 400},
		{"bad chunk size", func(a string) string {
			h := "POST https://example.invalid/ HTTP/1.1\r\nHost: example.invalid\r\n\r\n"
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: req-hdr=0, req-body=" + itoa(len(h)) + "\r\n\r\n" + h + "zz\r\nabc\r\n0\r\n\r\n"
		}, 400},
		{"chunk without CRLF", func(a string) string {
			h := "POST https://example.invalid/ HTTP/1.1\r\nHost: example.invalid\r\n\r\n"
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: req-hdr=0, req-body=" + itoa(len(h)) + "\r\n\r\n" + h + "3\r\nabcdef\r\n0\r\n\r\n"
		}, 400},
		{"preview longer than said", func(a string) string {
			h := "POST https://example.invalid/ HTTP/1.1\r\nHost: example.invalid\r\n\r\n"
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nPreview: 2\r\nEncapsulated: req-hdr=0, req-body=" + itoa(len(h)) + "\r\n\r\n" + h + "5\r\nabcde\r\n0\r\n\r\n"
		}, 400},
		{"folded header", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\n folded\r\nEncapsulated: null-body=0\r\n\r\n"
		}, 400},
		{"huge headers", func(a string) string {
			return "REQMOD icap://" + a + "/reqmod ICAP/1.0\r\nHost: x\r\nX-Big: " + strings.Repeat("a", 70*1024) + "\r\n\r\n"
		}, 400},
	}
	addr, _ := start(t, &fakeHandler{}, nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := dial(t, addr)
			if err := c.Write(tc.raw(addr)); err != nil {
				t.Fatal(err)
			}
			rep, err := c.Read()
			if err != nil {
				t.Fatal(err)
			}
			if rep.Status != tc.status {
				t.Fatalf("status %d, want %d", rep.Status, tc.status)
			}
			// A malformed message ends the connection.
			_ = c.C.SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, err := c.BR.ReadByte(); err != io.EOF {
				t.Fatalf("connection not closed: %v", err)
			}
		})
	}
}

func TestWrongServiceAndRespmod(t *testing.T) {
	addr, _ := start(t, &fakeHandler{}, nil)
	c := dial(t, addr)
	body := []byte(`{"q":"x"}`)
	rep, err := c.Do(icaptest.Request{Service: "/other", HTTP: post(string(body)), Body: body, Preview: -1})
	if err != nil || rep.Status != 404 {
		t.Fatalf("other service: %+v %v", rep, err)
	}
	h := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n"
	raw := "RESPMOD icap://" + addr + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: res-hdr=0, res-body=" + itoa(len(h)) + "\r\n\r\n" + h + icaptest.Chunked([]byte("answer"), 0, "")
	if err := c.Write(raw); err != nil {
		t.Fatal(err)
	}
	if rep, err := c.Read(); err != nil || rep.Status != 405 {
		t.Fatalf("RESPMOD: %+v %v", rep, err)
	}
	// Both kept the connection.
	if rep, err := c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: 0}); err != nil || rep.Status != 204 {
		t.Fatalf("after: %+v %v", rep, err)
	}
}

func TestConnectionClose(t *testing.T) {
	addr, _ := start(t, &fakeHandler{}, nil)
	c := dial(t, addr)
	body := []byte(`{"q":"x"}`)
	rep, err := c.Do(icaptest.Request{HTTP: post(string(body)), Body: body, Preview: 0, Header: map[string]string{"Connection": "close"}})
	if err != nil || rep.Status != 204 || rep.Header.Get("Connection") != "close" {
		t.Fatalf("%+v %v", rep, err)
	}
	_ = c.C.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.BR.ReadByte(); err != io.EOF {
		t.Fatalf("connection not closed: %v", err)
	}
}

func TestAllowClientAndMaxConnections(t *testing.T) {
	addr, _ := start(t, &fakeHandler{}, func(s *icap.Server) {
		s.AllowClient = func(ip net.IP) bool { return false }
	})
	c := dial(t, addr)
	_ = c.C.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.BR.ReadByte(); err != io.EOF {
		t.Fatalf("refused client not closed: %v", err)
	}

	addr, _ = start(t, &fakeHandler{}, func(s *icap.Server) { s.MaxConnections = 1 })
	first := dial(t, addr)
	// The first connection is in use once a reply has come back on it.
	if err := first.Write("OPTIONS icap://" + addr + "/reqmod ICAP/1.0\r\nHost: x\r\nEncapsulated: null-body=0\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Read(); err != nil {
		t.Fatal(err)
	}
	second := dial(t, addr)
	rep, err := second.Read()
	if err != nil || rep.Status != 503 {
		t.Fatalf("over Max-Connections: %+v %v", rep, err)
	}
}

func TestIdleTimeoutAndShutdown(t *testing.T) {
	addr, s := start(t, &fakeHandler{}, func(s *icap.Server) { s.IdleTimeout = 200 * time.Millisecond })
	c := dial(t, addr)
	_ = c.C.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.BR.ReadByte(); err != io.EOF {
		t.Fatalf("idle connection not closed: %v", err)
	}
	idle := dial(t, addr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = idle.C.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := idle.BR.ReadByte(); err != io.EOF {
		t.Fatalf("idle connection kept after shutdown: %v", err)
	}
}

func TestReadTimeoutMidMessage(t *testing.T) {
	addr, _ := start(t, &fakeHandler{}, func(s *icap.Server) { s.ReadTimeout = 200 * time.Millisecond })
	c := dial(t, addr)
	// Headers, then nothing.
	if err := c.Write("REQMOD icap://" + addr + "/reqmod ICAP/1.0\r\nHost: x\r\n"); err != nil {
		t.Fatal(err)
	}
	rep, err := c.Read()
	if err != nil || rep.Status != 408 {
		t.Fatalf("%+v %v", rep, err)
	}
}
