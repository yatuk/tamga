// Package icaptest is a small ICAP client for tests: it writes REQMOD and
// OPTIONS messages as a web proxy writes them, and reads the replies.
package icaptest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Conn is one connection to an ICAP server.
type Conn struct {
	C  net.Conn
	BR *bufio.Reader
}

// Dial connects to addr.
func Dial(addr string) (*Conn, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	return &Conn{C: c, BR: bufio.NewReader(c)}, nil
}

// Close closes the connection.
func (c *Conn) Close() error { return c.C.Close() }

// Write sends raw bytes.
func (c *Conn) Write(s string) error {
	_, err := io.WriteString(c.C, s)
	return err
}

// Reply is an ICAP reply as read.
type Reply struct {
	Status int
	Header http.Header
	// RawHTTP is the encapsulated header block, and Body the encapsulated
	// body, de-chunked. Request is set for req-hdr, Response for res-hdr.
	RawHTTP  []byte
	Body     []byte
	HasBody  bool
	Request  *http.Request
	Response *http.Response
}

// Request is a REQMOD to write.
type Request struct {
	// Service is the ICAP URI's path, "/reqmod" when empty.
	Service string
	// Header are ICAP headers beyond Host and Encapsulated.
	Header map[string]string
	// HTTP is the encapsulated request's header block, request line
	// included, without the empty line that ends it.
	HTTP string
	// Body is sent chunked in pieces of Chunk bytes (all at once when 0).
	// Nil sends null-body.
	Body  []byte
	Chunk int
	// Preview, when 0 or more, sends a Preview header and that many bytes
	// first; the rest is sent after a 100 Continue.
	Preview int
}

// Chunked encodes b as ICAP chunks of size n (one chunk when n is 0), the
// last chunk with ext ("; ieof" or "").
func Chunked(b []byte, n int, ext string) string {
	var sb strings.Builder
	if n <= 0 {
		n = len(b)
	}
	for len(b) > 0 {
		k := min(n, len(b))
		fmt.Fprintf(&sb, "%x\r\n", k)
		sb.Write(b[:k])
		sb.WriteString("\r\n")
		b = b[k:]
	}
	sb.WriteString("0" + ext + "\r\n\r\n")
	return sb.String()
}

// Head is the ICAP part of a REQMOD for r.
func Head(r Request, addr string) string {
	svc := r.Service
	if svc == "" {
		svc = "/reqmod"
	}
	httpHead := r.HTTP + "\r\n\r\n"
	enc := "req-hdr=0, null-body=" + strconv.Itoa(len(httpHead))
	if r.Body != nil {
		enc = "req-hdr=0, req-body=" + strconv.Itoa(len(httpHead))
	}
	var sb strings.Builder
	sb.WriteString("REQMOD icap://" + addr + svc + " ICAP/1.0\r\n")
	sb.WriteString("Host: " + addr + "\r\n")
	sb.WriteString("Encapsulated: " + enc + "\r\n")
	if r.Preview >= 0 && r.Body != nil {
		sb.WriteString("Preview: " + strconv.Itoa(r.Preview) + "\r\n")
	}
	for k, v := range r.Header {
		sb.WriteString(k + ": " + v + "\r\n")
	}
	sb.WriteString("\r\n")
	sb.WriteString(httpHead)
	return sb.String()
}

// Do sends r and reads the reply, answering a 100 Continue with the rest of
// the body. Set Preview to -1 for no preview.
func (c *Conn) Do(r Request) (*Reply, error) {
	addr := c.C.RemoteAddr().String()
	if err := c.Write(Head(r, addr)); err != nil {
		return nil, err
	}
	if r.Body == nil {
		return c.Read()
	}
	if r.Preview < 0 {
		if err := c.Write(Chunked(r.Body, r.Chunk, "")); err != nil {
			return nil, err
		}
		return c.Read()
	}
	p := min(r.Preview, len(r.Body))
	ext := ""
	if p == len(r.Body) {
		ext = "; ieof"
	}
	if err := c.Write(Chunked(r.Body[:p], r.Chunk, ext)); err != nil {
		return nil, err
	}
	rep, err := c.Read()
	if err != nil || rep.Status != 100 {
		return rep, err
	}
	if err := c.Write(Chunked(r.Body[p:], r.Chunk, "")); err != nil {
		return nil, err
	}
	return c.Read()
}

// Read reads one reply.
func (c *Conn) Read() (*Reply, error) {
	line, err := c.BR.ReadString('\n')
	if err != nil {
		return nil, err
	}
	f := strings.SplitN(strings.TrimRight(line, "\r\n"), " ", 3)
	if len(f) < 2 || f[0] != "ICAP/1.0" {
		return nil, fmt.Errorf("bad status line %q", line)
	}
	rep := &Reply{Header: http.Header{}}
	if rep.Status, err = strconv.Atoi(f[1]); err != nil {
		return nil, err
	}
	for {
		l, err := c.BR.ReadString('\n')
		if err != nil {
			return nil, err
		}
		l = strings.TrimRight(l, "\r\n")
		if l == "" {
			break
		}
		k, v, _ := strings.Cut(l, ":")
		rep.Header.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	if rep.Status == 100 {
		return rep, nil
	}
	enc := rep.Header.Get("Encapsulated")
	var hdrName, bodyName string
	var bodyOff int
	for _, p := range strings.Split(enc, ",") {
		n, o, _ := strings.Cut(strings.TrimSpace(p), "=")
		off, _ := strconv.Atoi(o)
		switch n {
		case "req-hdr", "res-hdr":
			hdrName = n
		default:
			bodyName, bodyOff = n, off
		}
	}
	if hdrName != "" {
		rep.RawHTTP = make([]byte, bodyOff)
		if _, err := io.ReadFull(c.BR, rep.RawHTTP); err != nil {
			return nil, err
		}
	}
	if bodyName == "req-body" || bodyName == "res-body" {
		rep.HasBody = true
		var body bytes.Buffer
		for {
			l, err := c.BR.ReadString('\n')
			if err != nil {
				return nil, err
			}
			sz, _, _ := strings.Cut(strings.TrimRight(l, "\r\n"), ";")
			n, err := strconv.ParseInt(strings.TrimSpace(sz), 16, 64)
			if err != nil {
				return nil, fmt.Errorf("bad chunk size %q", l)
			}
			if n == 0 {
				if _, err := c.BR.ReadString('\n'); err != nil {
					return nil, err
				}
				break
			}
			if _, err := io.CopyN(&body, c.BR, n); err != nil {
				return nil, err
			}
			if _, err := c.BR.ReadString('\n'); err != nil {
				return nil, err
			}
		}
		rep.Body = body.Bytes()
	}
	switch hdrName {
	case "req-hdr":
		rep.Request, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(rep.RawHTTP)))
	case "res-hdr":
		rep.Response, err = http.ReadResponse(bufio.NewReader(bytes.NewReader(rep.RawHTTP)), nil)
	}
	if err != nil {
		return nil, err
	}
	return rep, nil
}
