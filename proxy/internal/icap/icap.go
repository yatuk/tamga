// Package icap is an ICAP server (RFC 3507) for request modification: a web
// proxy that already intercepts its users' traffic sends a request to it,
// and gets back the request unchanged (204), changed, or an HTTP response to
// give the user in its place.
//
// It speaks the protocol and decides nothing. What a request is and what is
// done with it is the Handler's.
//
// What is implemented: OPTIONS and REQMOD, the Encapsulated header,
// chunked bodies with their extensions, Preview with ieof and 100 Continue,
// Allow: 204 and 204 after a preview, ISTag, persistent connections, and
// limits on the size of headers and bodies and on the time any step takes.
// RESPMOD is answered 405. A client that sends no Allow: 204 and no preview
// gets the request echoed back when it is not changed, as section 4.6 of
// the RFC requires.
package icap

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Defaults for the Server's limits.
const (
	DefaultMaxHeaderBytes = 64 * 1024
	DefaultMaxPreview     = 64 * 1024
	DefaultMaxConnections = 256
	DefaultIdleTimeout    = 60 * time.Second
	DefaultReadTimeout    = 30 * time.Second
	DefaultWriteTimeout   = 30 * time.Second
	DefaultAdaptTimeout   = 30 * time.Second
	// DefaultDrainBytes is how much of a body that is over the Handler's
	// limit is read and thrown away so that the connection can be used
	// again. A body larger than that ends the connection.
	DefaultDrainBytes = 32 * 1024 * 1024
)

// maxChunkLine is the longest chunk-size line read, extensions included.
const maxChunkLine = 4096

// Request is one REQMOD as the server read it.
type Request struct {
	// Header holds the ICAP headers, with canonical names.
	Header http.Header
	// HTTP is the encapsulated request, headers only. Its Body is empty;
	// the body is in Body.
	HTTP *http.Request
	// RawHTTP is the encapsulated request's header block as it came,
	// request line included and the empty line that ends it.
	RawHTTP []byte
	// Body is the request body, de-chunked, as far as it was read.
	Body []byte
	// HasBody is false for a request the client sent with null-body.
	HasBody bool
	// BodyTooLarge says the body was longer than the limit the Handler set
	// in Inspect. Body is then empty and Adapt is asked for a refusal.
	BodyTooLarge bool
	// RemoteAddr is the address of the ICAP client: the web proxy, not the
	// user.
	RemoteAddr string
}

// ResultKind says what becomes of a request.
type ResultKind int

const (
	// Unmodified lets the request go on as it came.
	Unmodified ResultKind = iota
	// Modified sends it on with Result.Body in place of its body.
	Modified
	// Respond answers it with Result's response. It is not sent on.
	Respond
)

// Result is what a Handler decides about a request.
type Result struct {
	Kind ResultKind
	// Body is the new body of a Modified request. Content-Length is set
	// to its length; Transfer-Encoding is removed.
	Body []byte
	// DropHeaders are request headers removed from a Modified request, for
	// example Content-Encoding when Body is the decoded form.
	DropHeaders []string
	// Status, ResponseHeader and ResponseBody are the HTTP response of a
	// Respond. Content-Length is set from ResponseBody.
	Status         int
	ResponseHeader http.Header
	ResponseBody   []byte
}

// Handler decides about requests.
type Handler interface {
	// Inspect is asked once the encapsulated headers (and a preview, if
	// any) are read. want false lets the request go on unread. limit is the
	// largest body Adapt will be given; a longer one reaches Adapt with
	// BodyTooLarge set.
	Inspect(req *Request) (want bool, limit int)
	// Adapt is given a request Inspect wanted, with its body.
	Adapt(ctx context.Context, req *Request) Result
	// ISTag names the service's current state (RFC 3507, 4.7): it changes
	// when what Adapt would decide may have changed. At most 32 characters
	// of [A-Za-z0-9._-]; anything else is dropped.
	ISTag() string
}

// Server serves ICAP on a listener.
type Server struct {
	Handler Handler
	// Service is the path of the one service, "/reqmod" when empty.
	Service string
	// AllowClient, when set, is asked about each connection's peer. A
	// connection it refuses is closed without a reply.
	AllowClient func(ip net.IP) bool
	// OnError, when set, is told about connections that ended in an error
	// and about refused peers. The error never holds request content.
	OnError func(remote string, err error)

	MaxConnections int
	MaxHeaderBytes int
	MaxPreview     int
	DrainBytes     int
	IdleTimeout    time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	AdaptTimeout   time.Duration

	mu       sync.Mutex
	ln       net.Listener
	conns    map[net.Conn]bool // true while the connection waits for a message
	closing  bool
	wg       sync.WaitGroup
	slots    chan struct{}
	initOnce sync.Once
}

func (s *Server) init() {
	s.initOnce.Do(func() {
		if s.Service == "" {
			s.Service = "/reqmod"
		}
		if s.MaxConnections <= 0 {
			s.MaxConnections = DefaultMaxConnections
		}
		if s.MaxHeaderBytes <= 0 {
			s.MaxHeaderBytes = DefaultMaxHeaderBytes
		}
		if s.MaxPreview <= 0 {
			s.MaxPreview = DefaultMaxPreview
		}
		if s.DrainBytes <= 0 {
			s.DrainBytes = DefaultDrainBytes
		}
		if s.IdleTimeout <= 0 {
			s.IdleTimeout = DefaultIdleTimeout
		}
		if s.ReadTimeout <= 0 {
			s.ReadTimeout = DefaultReadTimeout
		}
		if s.WriteTimeout <= 0 {
			s.WriteTimeout = DefaultWriteTimeout
		}
		if s.AdaptTimeout <= 0 {
			s.AdaptTimeout = DefaultAdaptTimeout
		}
		s.slots = make(chan struct{}, s.MaxConnections)
		s.conns = map[net.Conn]bool{}
	})
}

// ErrServerClosed is returned by Serve after Shutdown.
var ErrServerClosed = errors.New("icap: server closed")

// Serve accepts connections on ln until Shutdown is called.
func (s *Server) Serve(ln net.Listener) error {
	s.init()
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return ErrServerClosed
	}
	s.ln = ln
	s.mu.Unlock()
	for {
		c, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			if closing {
				return ErrServerClosed
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return err
		}
		if s.AllowClient != nil {
			var ip net.IP
			if ta, ok := c.RemoteAddr().(*net.TCPAddr); ok {
				ip = ta.IP
			}
			if ip == nil || !s.AllowClient(ip) {
				s.report(c.RemoteAddr().String(), errors.New("client address not allowed"))
				_ = c.Close()
				continue
			}
		}
		select {
		case s.slots <- struct{}{}:
		default:
			// Over Max-Connections: told so and closed.
			_ = c.SetWriteDeadline(time.Now().Add(time.Second))
			_, _ = io.WriteString(c, "ICAP/1.0 503 Service Overloaded\r\nConnection: close\r\nEncapsulated: null-body=0\r\n\r\n")
			_ = c.Close()
			continue
		}
		s.mu.Lock()
		s.conns[c] = true
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer func() {
				s.mu.Lock()
				delete(s.conns, c)
				s.mu.Unlock()
				_ = c.Close()
				<-s.slots
				s.wg.Done()
			}()
			s.serveConn(c)
		}()
	}
}

// Shutdown stops accepting, closes idle connections and waits for the
// others to finish their message, or for ctx.
func (s *Server) Shutdown(ctx context.Context) error {
	s.init()
	s.mu.Lock()
	s.closing = true
	if s.ln != nil {
		_ = s.ln.Close()
	}
	// A connection waiting for its next message is woken now; one in the
	// middle of a message finishes it (its deadlines bound that) and is
	// not given another.
	for c, idle := range s.conns {
		if idle {
			_ = c.SetReadDeadline(time.Now())
		}
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		for c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
		return ctx.Err()
	}
}

func (s *Server) report(remote string, err error) {
	if s.OnError != nil {
		s.OnError(remote, err)
	}
}

func (s *Server) setIdle(c net.Conn, idle bool) {
	s.mu.Lock()
	s.conns[c] = idle
	s.mu.Unlock()
}

func (s *Server) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closing
}

// protoError is a request the server answers with an ICAP error status and
// then closes the connection on: after a malformed message the stream can
// no longer be trusted to be at a message boundary.
type protoError struct {
	status int
	msg    string
}

func (e *protoError) Error() string { return strconv.Itoa(e.status) + " " + e.msg }

func bad(format string, a ...interface{}) error {
	return &protoError{status: 400, msg: fmt.Sprintf(format, a...)}
}

var statusText = map[int]string{
	100: "Continue",
	200: "OK",
	204: "No Content",
	400: "Bad Request",
	404: "ICAP Service Not Found",
	405: "Method Not Allowed For Service",
	408: "Request Timeout",
	500: "Server Error",
	501: "Method Not Implemented",
	503: "Service Overloaded",
	505: "ICAP Version Not Supported",
}

func (s *Server) serveConn(c net.Conn) {
	br := bufio.NewReaderSize(c, 16*1024)
	bw := bufio.NewWriterSize(c, 16*1024)
	remote := c.RemoteAddr().String()
	for {
		// Idle first, then the check: a Shutdown in between finds the
		// connection idle and wakes it.
		s.setIdle(c, true)
		if s.isClosing() {
			return
		}
		_ = c.SetReadDeadline(time.Now().Add(s.IdleTimeout))
		// Wait for the first byte of the next message under the idle
		// timeout; the message itself is then read under ReadTimeout.
		if _, err := br.Peek(1); err != nil {
			return
		}
		s.setIdle(c, false)
		_ = c.SetReadDeadline(time.Now().Add(s.ReadTimeout))
		keep, err := s.serveMessage(c, br, bw, remote)
		if err != nil {
			var pe *protoError
			if errors.As(err, &pe) {
				s.writeStatus(c, bw, pe.status, nil, true)
			} else {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					s.writeStatus(c, bw, 408, nil, true)
				}
			}
			s.report(remote, err)
			return
		}
		if !keep {
			return
		}
	}
}

// writeStatus writes a reply with no encapsulated message.
func (s *Server) writeStatus(c net.Conn, bw *bufio.Writer, status int, extra [][2]string, closeConn bool) {
	_ = c.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
	s.writeHead(bw, status, extra, "null-body=0", closeConn)
	_ = bw.Flush()
}

func (s *Server) writeHead(bw *bufio.Writer, status int, extra [][2]string, encapsulated string, closeConn bool) {
	fmt.Fprintf(bw, "ICAP/1.0 %d %s\r\n", status, statusText[status])
	if status != 100 {
		fmt.Fprintf(bw, "ISTag: \"%s\"\r\n", cleanISTag(s.Handler.ISTag()))
		bw.WriteString("Server: Tamga\r\n")
		fmt.Fprintf(bw, "Date: %s\r\n", time.Now().UTC().Format(http.TimeFormat))
		for _, h := range extra {
			bw.WriteString(h[0] + ": " + h[1] + "\r\n")
		}
		if closeConn {
			bw.WriteString("Connection: close\r\n")
		}
		bw.WriteString("Encapsulated: " + encapsulated + "\r\n")
	}
	bw.WriteString("\r\n")
}

// cleanISTag keeps the characters an ISTag may carry, and at most 32.
func cleanISTag(tag string) string {
	var b strings.Builder
	for _, r := range tag {
		if b.Len() == 32 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "tamga"
	}
	return b.String()
}

// encapsulated is a parsed Encapsulated header.
type encapsulated struct {
	names   []string
	offsets []int
}

func (e encapsulated) offset(name string) (int, bool) {
	for i, n := range e.names {
		if n == name {
			return e.offsets[i], true
		}
	}
	return 0, false
}

// parseEncapsulated reads "req-hdr=0, req-body=412": entities in the order
// the RFC gives them, offsets from 0 and increasing, a body entity (or
// null-body) last.
func parseEncapsulated(v string) (encapsulated, error) {
	var e encapsulated
	if strings.TrimSpace(v) == "" {
		return e, bad("empty Encapsulated header")
	}
	for _, part := range strings.Split(v, ",") {
		name, off, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return e, bad("malformed Encapsulated header")
		}
		name = strings.ToLower(strings.TrimSpace(name))
		switch name {
		case "req-hdr", "res-hdr", "req-body", "res-body", "null-body", "opt-body":
		default:
			return e, bad("unknown entity in Encapsulated header")
		}
		n, err := strconv.Atoi(strings.TrimSpace(off))
		if err != nil || n < 0 {
			return e, bad("malformed offset in Encapsulated header")
		}
		if _, dup := e.offset(name); dup {
			return e, bad("repeated entity in Encapsulated header")
		}
		if len(e.offsets) == 0 && n != 0 {
			return e, bad("first Encapsulated offset is not 0")
		}
		if len(e.offsets) > 0 && n < e.offsets[len(e.offsets)-1] {
			return e, bad("Encapsulated offsets are not in order")
		}
		e.names = append(e.names, name)
		e.offsets = append(e.offsets, n)
	}
	for i, n := range e.names {
		isBody := strings.HasSuffix(n, "-body")
		if isBody != (i == len(e.names)-1) {
			return e, bad("Encapsulated header must end with exactly one body entity")
		}
	}
	return e, nil
}

// readLine reads one CRLF-terminated line of at most max bytes, without the
// line ending. A bare LF is accepted as the end of a line.
func readLine(br *bufio.Reader, max int) (string, error) {
	var buf []byte
	for {
		frag, err := br.ReadSlice('\n')
		buf = append(buf, frag...)
		if len(buf) > max {
			return "", bad("line too long")
		}
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(buf) > 0 {
			return "", io.ErrUnexpectedEOF
		}
		return "", err
	}
	buf = bytes.TrimSuffix(buf, []byte("\n"))
	buf = bytes.TrimSuffix(buf, []byte("\r"))
	return string(buf), nil
}

// readHeaders reads header lines up to the empty line, at most max bytes in
// all. Folded lines are refused.
func readHeaders(br *bufio.Reader, max int) (http.Header, error) {
	h := http.Header{}
	total := 0
	for {
		line, err := readLine(br, max)
		if err != nil {
			return nil, err
		}
		total += len(line) + 2
		if total > max {
			return nil, bad("headers too large")
		}
		if line == "" {
			return h, nil
		}
		if line[0] == ' ' || line[0] == '\t' {
			return nil, bad("folded header line")
		}
		name, value, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			return nil, bad("malformed header line")
		}
		h.Add(name, strings.TrimSpace(value))
	}
}

// chunkReader reads an ICAP chunked body.
type chunkReader struct {
	br *bufio.Reader
}

// next reads one chunk into dst, or, when discard is set, throws it away.
// last is true for the zero-length chunk, and ieof is its extension.
func (r chunkReader) next(dst *bytes.Buffer, room int, discard bool) (n int, last, ieof, over bool, err error) {
	line, err := readLine(r.br, maxChunkLine)
	if err != nil {
		return 0, false, false, false, err
	}
	sizeText, ext, _ := strings.Cut(line, ";")
	sizeText = strings.TrimSpace(sizeText)
	if sizeText == "" || len(sizeText) > 15 {
		return 0, false, false, false, bad("malformed chunk size")
	}
	size, perr := strconv.ParseInt(sizeText, 16, 64)
	if perr != nil || size < 0 {
		return 0, false, false, false, bad("malformed chunk size")
	}
	if size == 0 {
		for _, e := range strings.Split(ext, ";") {
			if strings.EqualFold(strings.TrimSpace(e), "ieof") {
				ieof = true
			}
		}
		// Trailer lines, which ICAP has no use for, up to the empty line.
		if _, err := readHeaders(r.br, maxChunkLine); err != nil {
			return 0, true, ieof, false, err
		}
		return 0, true, ieof, false, nil
	}
	if !discard && int64(room) < size {
		// The chunk is read past, so that the rest can be drained.
		over, discard = true, true
		if size > DefaultDrainBytes {
			return 0, false, false, false, bad("chunk too large")
		}
	}
	if discard {
		if _, err := io.CopyN(io.Discard, r.br, size); err != nil {
			return 0, false, false, false, unexpected(err)
		}
	} else if _, err := io.CopyN(dst, r.br, size); err != nil {
		return 0, false, false, false, unexpected(err)
	}
	if crlf, err := readLine(r.br, 2); err != nil || crlf != "" {
		if err == nil {
			err = bad("chunk data not followed by CRLF")
		}
		return 0, false, false, false, err
	}
	return int(size), false, false, over, nil
}

func unexpected(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// readBody reads chunks up to and including the last one. It stops early,
// with over set, when the next chunk does not fit in room.
func (r chunkReader) readBody(dst *bytes.Buffer, room int) (ieof, over bool, err error) {
	for {
		n, last, ie, ov, err := r.next(dst, room, false)
		if err != nil {
			return false, false, err
		}
		if ov {
			return false, true, nil
		}
		if last {
			return ie, false, nil
		}
		room -= n
	}
}

// drain throws away chunks up to the last one, at most max bytes of them.
func (r chunkReader) drain(max int) (ok bool, err error) {
	for {
		n, last, _, _, err := r.next(nil, 0, true)
		if err != nil {
			return false, err
		}
		if last {
			return true, nil
		}
		if max -= n; max < 0 {
			return false, nil
		}
	}
}

// serveMessage reads and answers one message. keep says the connection may
// carry another.
func (s *Server) serveMessage(c net.Conn, br *bufio.Reader, bw *bufio.Writer, remote string) (keep bool, err error) {
	line, err := readLine(br, s.MaxHeaderBytes)
	if err != nil {
		return false, err
	}
	parts := strings.Split(line, " ")
	if len(parts) != 3 {
		return false, bad("malformed request line")
	}
	method, rawURI, version := parts[0], parts[1], parts[2]
	if !strings.HasPrefix(version, "ICAP/") {
		return false, bad("malformed request line")
	}
	if version != "ICAP/1.0" {
		return false, &protoError{status: 505, msg: "unsupported version"}
	}
	hdr, err := readHeaders(br, s.MaxHeaderBytes)
	if err != nil {
		return false, err
	}
	keep = !strings.EqualFold(strings.TrimSpace(hdr.Get("Connection")), "close")

	u, perr := url.Parse(rawURI)
	if perr != nil || (u.Scheme != "icap" && u.Scheme != "icaps") || u.Host == "" {
		return false, bad("malformed ICAP URI")
	}
	if hdr.Get("Host") == "" {
		return false, bad("missing Host header")
	}
	enc, err := parseEncapsulated(hdr.Get("Encapsulated"))
	if err != nil {
		if method != "OPTIONS" || hdr.Get("Encapsulated") != "" {
			return false, err
		}
		enc = encapsulated{names: []string{"null-body"}, offsets: []int{0}}
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path != s.Service {
		// The message is read past before the 404, so the connection can
		// stay open.
		if err := s.skip(br, enc); err != nil {
			return false, err
		}
		s.writeStatus(c, bw, 404, nil, !keep)
		return keep, nil
	}

	switch method {
	case "OPTIONS":
		if _, ok := enc.offset("null-body"); !ok || len(enc.names) != 1 {
			return false, bad("OPTIONS with a body is not supported")
		}
		s.writeStatus(c, bw, 200, [][2]string{
			{"Methods", "REQMOD"},
			{"Service", "Tamga request scanning"},
			{"Max-Connections", strconv.Itoa(s.MaxConnections)},
			{"Options-TTL", "300"},
			{"Allow", "204"},
			// Nothing but the headers is needed to tell whether a request
			// is one Tamga reads.
			{"Preview", "0"},
			{"Transfer-Preview", "*"},
		}, !keep)
		return keep, nil
	case "REQMOD":
		return s.reqmod(c, br, bw, hdr, enc, remote, keep)
	case "RESPMOD":
		if err := s.skip(br, enc); err != nil {
			return false, err
		}
		s.writeStatus(c, bw, 405, nil, !keep)
		return keep, nil
	default:
		return false, &protoError{status: 501, msg: "method not implemented"}
	}
}

// skip reads past the encapsulated part of a message that is refused.
func (s *Server) skip(br *bufio.Reader, enc encapsulated) error {
	last := len(enc.names) - 1
	hdrLen := enc.offsets[last]
	if hdrLen > s.MaxHeaderBytes {
		return bad("encapsulated headers too large")
	}
	if _, err := io.CopyN(io.Discard, br, int64(hdrLen)); err != nil {
		return unexpected(err)
	}
	if enc.names[last] == "null-body" {
		return nil
	}
	ok, err := chunkReader{br}.drain(s.DrainBytes)
	if err != nil {
		return err
	}
	if !ok {
		return bad("body too large")
	}
	return nil
}

func (s *Server) reqmod(c net.Conn, br *bufio.Reader, bw *bufio.Writer, hdr http.Header, enc encapsulated, remote string, keep bool) (bool, error) {
	reqHdr, hasHdr := enc.offset("req-hdr")
	if !hasHdr || reqHdr != 0 {
		return false, bad("REQMOD without req-hdr")
	}
	for _, n := range enc.names {
		if n != "req-hdr" && n != "req-body" && n != "null-body" {
			return false, bad("REQMOD with an entity other than req-hdr and req-body")
		}
	}
	bodyName := enc.names[len(enc.names)-1]
	hdrLen := enc.offsets[len(enc.offsets)-1]
	if hdrLen > s.MaxHeaderBytes {
		return false, bad("encapsulated headers too large")
	}
	raw := make([]byte, hdrLen)
	if _, err := io.ReadFull(br, raw); err != nil {
		return false, unexpected(err)
	}
	if !bytes.HasSuffix(raw, []byte("\r\n\r\n")) {
		return false, bad("encapsulated headers do not end with an empty line")
	}
	hreq, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return false, bad("malformed encapsulated request")
	}
	hreq.Body = http.NoBody
	req := &Request{Header: hdr, HTTP: hreq, RawHTTP: raw, HasBody: bodyName == "req-body", RemoteAddr: remote}
	allow204 := false
	for _, v := range hdr.Values("Allow") {
		for _, a := range strings.Split(v, ",") {
			if strings.TrimSpace(a) == "204" {
				allow204 = true
			}
		}
	}

	cr := chunkReader{br}
	var body bytes.Buffer
	preview := -1
	if pv := hdr.Get("Preview"); pv != "" {
		n, err := strconv.Atoi(strings.TrimSpace(pv))
		if err != nil || n < 0 {
			return false, bad("malformed Preview header")
		}
		preview = n
	}
	complete := !req.HasBody
	if req.HasBody && preview >= 0 {
		if preview > s.MaxPreview {
			return false, bad("preview too large")
		}
		ieof, over, err := cr.readBody(&body, preview)
		if err != nil {
			return false, err
		}
		if over {
			return false, bad("preview longer than the Preview header says")
		}
		complete = ieof
	}
	req.Body = body.Bytes()

	want, limit := s.Handler.Inspect(req)
	if !want {
		switch {
		case preview >= 0 || allow204:
			// After a preview a 204 is always allowed (RFC 3507, 4.5). A
			// body that was not all sent is not read: the client sends it
			// on itself.
			if !complete && preview < 0 {
				if ok, err := cr.drain(s.DrainBytes); err != nil || !ok {
					if err == nil {
						err = bad("body too large")
					}
					return false, err
				}
			}
			s.writeStatus(c, bw, 204, nil, !keep)
			return keep, nil
		default:
			return s.echo(c, bw, cr, req, !complete, keep)
		}
	}

	if !complete {
		if preview >= 0 {
			_ = c.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
			s.writeHead(bw, 100, nil, "", false)
			if err := bw.Flush(); err != nil {
				return false, err
			}
			_ = c.SetReadDeadline(time.Now().Add(s.ReadTimeout))
		}
		room := limit - body.Len()
		if room < 0 {
			room = 0
		}
		_, over, err := cr.readBody(&body, room)
		if err != nil {
			return false, err
		}
		if over {
			req.BodyTooLarge = true
			ok, err := cr.drain(s.DrainBytes)
			if err != nil {
				return false, err
			}
			keep = keep && ok
		}
	}
	if body.Len() > limit {
		req.BodyTooLarge = true
	}
	if req.BodyTooLarge {
		req.Body = nil
	} else {
		req.Body = body.Bytes()
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.AdaptTimeout)
	res := s.Handler.Adapt(ctx, req)
	cancel()
	echo := res.Kind == Unmodified && !allow204 && preview < 0
	if req.BodyTooLarge && (echo || res.Kind == Modified && res.Body == nil) {
		// The body was not kept, so it cannot be sent back.
		return false, &protoError{status: 500, msg: "no body to send back"}
	}
	_ = c.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
	switch {
	case echo:
		entity := "null-body"
		if req.HasBody {
			entity = "req-body"
		}
		s.writeHead(bw, 200, nil, "req-hdr=0, "+entity+"="+strconv.Itoa(len(raw)), !keep)
		bw.Write(raw)
		if req.HasBody {
			writeChunk(bw, req.Body)
			bw.WriteString("0\r\n\r\n")
		}
	case res.Kind == Unmodified:
		s.writeHead(bw, 204, nil, "null-body=0", !keep)
	case res.Kind == Modified:
		head := rewriteHead(raw, len(res.Body), res.DropHeaders)
		writeEncapsulated(s, bw, "req", head, res.Body, true, keep)
	default:
		head, b := responseHead(res)
		writeEncapsulated(s, bw, "res", head, b, len(b) > 0, keep)
	}
	if err := bw.Flush(); err != nil {
		return false, err
	}
	return keep, nil
}

// echo answers a request that passes unchanged to a client that allowed no
// 204: the request goes back as it came, the rest of its body copied chunk
// by chunk as it is read.
func (s *Server) echo(c net.Conn, bw *bufio.Writer, cr chunkReader, req *Request, more, keep bool) (bool, error) {
	_ = c.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
	if !req.HasBody {
		s.writeHead(bw, 200, nil, "req-hdr=0, null-body="+strconv.Itoa(len(req.RawHTTP)), !keep)
		bw.Write(req.RawHTTP)
		return keep, bw.Flush()
	}
	s.writeHead(bw, 200, nil, "req-hdr=0, req-body="+strconv.Itoa(len(req.RawHTTP)), !keep)
	bw.Write(req.RawHTTP)
	writeChunk(bw, req.Body)
	var buf bytes.Buffer
	for more {
		buf.Reset()
		_ = c.SetReadDeadline(time.Now().Add(s.ReadTimeout))
		_, last, _, _, err := cr.next(&buf, 1<<30, false)
		if err != nil {
			return false, err
		}
		if last {
			break
		}
		_ = c.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
		writeChunk(bw, buf.Bytes())
	}
	bw.WriteString("0\r\n\r\n")
	return keep, bw.Flush()
}

func writeChunk(bw *bufio.Writer, b []byte) {
	if len(b) == 0 {
		return
	}
	fmt.Fprintf(bw, "%x\r\n", len(b))
	bw.Write(b)
	bw.WriteString("\r\n")
}

// writeEncapsulated writes a 200 that carries an HTTP message: the request
// ("req") or the response ("res") head, and the body, chunked.
func writeEncapsulated(s *Server, bw *bufio.Writer, kind string, head, body []byte, hasBody, keep bool) {
	bodyEntity := "null-body"
	if hasBody {
		bodyEntity = kind + "-body"
	}
	s.writeHead(bw, 200, nil, kind+"-hdr=0, "+bodyEntity+"="+strconv.Itoa(len(head)), !keep)
	bw.Write(head)
	if hasBody {
		writeChunk(bw, body)
		bw.WriteString("0\r\n\r\n")
	}
}

// rewriteHead is the request's header block with Content-Length set to n,
// Transfer-Encoding removed and drop removed. Everything else stays as it
// came, in its order.
func rewriteHead(raw []byte, n int, drop []string) []byte {
	lines := strings.Split(strings.TrimSuffix(string(raw), "\r\n\r\n"), "\r\n")
	var b strings.Builder
	b.WriteString(lines[0] + "\r\n")
	for _, l := range lines[1:] {
		name, _, _ := strings.Cut(l, ":")
		name = strings.TrimSpace(name)
		if strings.EqualFold(name, "Content-Length") || strings.EqualFold(name, "Transfer-Encoding") {
			continue
		}
		dropped := false
		for _, d := range drop {
			if strings.EqualFold(name, d) {
				dropped = true
			}
		}
		if !dropped {
			b.WriteString(l + "\r\n")
		}
	}
	b.WriteString("Content-Length: " + strconv.Itoa(n) + "\r\n\r\n")
	return []byte(b.String())
}

// responseHead is the HTTP response a Respond result is sent as.
func responseHead(res Result) (head, body []byte) {
	status := res.Status
	if status == 0 {
		status = http.StatusForbidden
	}
	var b strings.Builder
	fmt.Fprintf(&b, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
	h := res.ResponseHeader.Clone()
	if h == nil {
		h = http.Header{}
	}
	h.Del("Transfer-Encoding")
	h.Set("Content-Length", strconv.Itoa(len(res.ResponseBody)))
	_ = h.Write(&b)
	b.WriteString("\r\n")
	return []byte(b.String()), res.ResponseBody
}
