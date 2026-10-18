package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// tamga mcp-wrap runs a local MCP server that speaks the stdio transport and
// serves it over Streamable HTTP, so that the MCP gateway (/mcp/<name>) can
// stand in front of it like any remote server:
//
//	tamga mcp-wrap -listen 127.0.0.1:8931 -- npx -y some-mcp-server
//
// and in TAMGA_MCP_CONFIG: url: http://127.0.0.1:8931/mcp
//
// It is a small bridge, not a server of its own. Each POST carries one
// JSON-RPC message; a request is written to the child's stdin with an id of
// the bridge's own, so that two clients cannot mix up their answers, and
// the child's response is returned as one JSON object with the client's id
// back in place. A notification is written and answered 202. There is no
// GET stream and no session: what the child sends that answers no request
// (its notifications, and in the 2025 revisions its own requests such as
// sampling) is dropped and counted on stderr. When the HTTP client goes
// away first, the child is sent notifications/cancelled for the request.
//
// The stdio transport is in the MCP specification, revision 2026-07-28,
// basic/transports/stdio: newline-delimited JSON-RPC on stdin and stdout,
// logs on stderr, which is passed through.

const (
	mcpWrapMaxMessage = 4 << 20
	mcpWrapMaxLine    = 32 << 20
)

func runMCPWrap(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcp-wrap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:8931", "address to serve Streamable HTTP on")
	path := fs.String("path", "/mcp", "path of the MCP endpoint")
	timeout := fs.Duration("timeout", 5*time.Minute, "how long one request may wait for the server's answer")
	remote := fs.Bool("allow-remote", false, "allow a listen address that is not loopback")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tamga mcp-wrap [flags] -- command [args...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	command := fs.Args()
	if len(command) == 0 {
		fs.Usage()
		return 2
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		fmt.Fprintln(stderr, "mcp-wrap: -listen must be host:port")
		return 2
	}
	if ip := net.ParseIP(host); !*remote && (ip == nil || !ip.IsLoopback()) && host != "localhost" {
		// The transport asks a local server to listen on loopback only. The
		// bridge has no authentication of its own; Tamga in front of it has.
		fmt.Fprintln(stderr, "mcp-wrap: -listen is not a loopback address; pass -allow-remote to listen there anyway")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stderr = stderr
	b, err := startStdioBridge(cmd, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "mcp-wrap:", err)
		return 1
	}
	b.timeout = *timeout

	mux := http.NewServeMux()
	mux.Handle(*path, b)
	srv := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-b.done
		_ = srv.Close()
	}()
	fmt.Fprintf(stderr, "mcp-wrap: serving %s on http://%s%s\n", command[0], *listen, *path)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, "mcp-wrap:", err)
		_ = cmd.Process.Kill()
		return 1
	}
	if code := cmd.ProcessState; code != nil {
		return code.ExitCode()
	}
	return 0
}

// stdioBridge is one child process and the requests waiting on it.
type stdioBridge struct {
	stdin   io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan []byte
	next    atomic.Uint64
	done    chan struct{}
	dropped atomic.Uint64
	timeout time.Duration
	log     io.Writer
}

func startStdioBridge(cmd *exec.Cmd, logw io.Writer) (*stdioBridge, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	b := &stdioBridge{stdin: stdin, pending: map[string]chan []byte{}, done: make(chan struct{}), timeout: 5 * time.Minute, log: logw}
	go func() {
		b.read(stdout)
		_ = cmd.Wait()
		close(b.done)
	}()
	return b, nil
}

// read takes the child's messages, one per line, and hands each response to
// the request waiting for it.
func (b *stdioBridge) read(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64<<10), mcpWrapMaxLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method *string         `json:"method"`
		}
		key := ""
		if json.Unmarshal(line, &msg) == nil && msg.Method == nil {
			key = string(msg.ID)
		}
		b.mu.Lock()
		ch := b.pending[key]
		delete(b.pending, key)
		b.mu.Unlock()
		if ch == nil {
			if n := b.dropped.Add(1); n == 1 || n%100 == 0 {
				fmt.Fprintf(b.log, "mcp-wrap: %d message(s) from the server answered no request and were dropped\n", n)
			}
			continue
		}
		ch <- append([]byte(nil), line...)
	}
}

func (b *stdioBridge) write(line []byte) error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	_, err := b.stdin.Write(append(line, '\n'))
	return err
}

func wrapError(w http.ResponseWriter, status int, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":null,"error":{"code":%d,"message":%q}}`, code, msg)
}

func (b *stdioBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// No web page may call a local server (DNS rebinding).
	if o := strings.ToLower(r.Header.Get("Origin")); strings.HasPrefix(o, "http://") || strings.HasPrefix(o, "https://") {
		wrapError(w, http.StatusForbidden, -32600, "origin not allowed")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mcpWrapMaxMessage))
	if err != nil {
		wrapError(w, http.StatusRequestEntityTooLarge, -32600, "message too large")
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var msg map[string]any
	if err := dec.Decode(&msg); err != nil || msg == nil {
		wrapError(w, http.StatusBadRequest, -32700, "a POST carries one JSON-RPC message")
		return
	}
	id, hasID := msg["id"]
	_, isRequest := msg["method"]
	if !hasID || !isRequest {
		// A notification, or a response to a request the bridge never passes.
		line, _ := json.Marshal(msg)
		if err := b.write(line); err != nil {
			wrapError(w, http.StatusBadGateway, -32603, "the server is not running")
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	own := "tamga-wrap-" + strconv.FormatUint(b.next.Add(1), 10)
	key, _ := json.Marshal(own)
	ch := make(chan []byte, 1)
	b.mu.Lock()
	b.pending[string(key)] = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, string(key))
		b.mu.Unlock()
	}()
	msg["id"] = own
	line, _ := json.Marshal(msg)
	if err := b.write(line); err != nil {
		wrapError(w, http.StatusBadGateway, -32603, "the server is not running")
		return
	}

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	select {
	case resp := <-ch:
		var out map[string]any
		d := json.NewDecoder(bytes.NewReader(resp))
		d.UseNumber()
		if err := d.Decode(&out); err != nil {
			wrapError(w, http.StatusBadGateway, -32603, "the server sent a message that is not JSON")
			return
		}
		out["id"] = id
		body, _ := json.Marshal(out)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case <-r.Context().Done():
		b.cancel(own)
	case <-timer.C:
		b.cancel(own)
		wrapError(w, http.StatusGatewayTimeout, -32603, "the server did not answer in time")
	case <-b.done:
		wrapError(w, http.StatusBadGateway, -32603, "the server exited")
	}
}

// cancel tells the child that nobody waits for the request any more.
func (b *stdioBridge) cancel(id string) {
	line, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params":  map[string]any{"requestId": id, "reason": "the HTTP client went away"},
	})
	_ = b.write(line)
}
