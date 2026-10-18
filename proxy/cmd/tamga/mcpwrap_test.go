package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// The child is this test binary, run as a stdio MCP server: it answers
// every request with its method and id, and writes a notification before
// each answer, which the bridge drops.
func TestMain(m *testing.M) {
	if os.Getenv("TAMGA_MCPWRAP_CHILD") == "1" {
		stdioChild()
		return
	}
	os.Exit(m.Run())
}

func stdioChild() {
	sc := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var msg map[string]any
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		id, ok := msg["id"]
		if !ok {
			continue
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/message", "params": map[string]any{"level": "info"}})
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"method": msg["method"], "seen_id": id}})
	}
}

func TestMCPWrapBridgesStdio(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "TAMGA_MCPWRAP_CHILD=1")
	b, err := startStdioBridge(cmd, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.stdin.Close(); <-b.done }()
	srv := httptest.NewServer(b)
	defer srv.Close()

	post := func(body string, origin string) (*http.Response, map[string]any) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp, out
	}

	// Two clients that use the same id each get their own answer, with
	// their id back.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method := fmt.Sprintf("tools/list-%d", i)
			resp, out := post(`{"jsonrpc":"2.0","id":1,"method":"`+method+`"}`, "")
			result, _ := out["result"].(map[string]any)
			if resp.StatusCode != http.StatusOK || out["id"] != float64(1) || result["method"] != method {
				t.Errorf("client %d: %d %v", i, resp.StatusCode, out)
			}
			if seen, _ := result["seen_id"].(string); !strings.HasPrefix(seen, "tamga-wrap-") {
				t.Errorf("the child saw the client's id: %v", result["seen_id"])
			}
		}(i)
	}
	wg.Wait()

	if resp, _ := post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, ""); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notification: %d", resp.StatusCode)
	}
	if resp, _ := post(`{"jsonrpc":"2.0","id":2,"method":"x"}`, "https://evil.example"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("origin: %d", resp.StatusCode)
	}
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	if b.dropped.Load() == 0 {
		t.Fatal("the child's notifications were not counted as dropped")
	}
}

func TestMCPWrapRefusesRemoteListen(t *testing.T) {
	if code := runMCPWrap([]string{"-listen", "0.0.0.0:0", "--", "true"}, io.Discard); code != 2 {
		t.Fatalf("exit code %d", code)
	}
	if code := runMCPWrap(nil, io.Discard); code != 2 {
		t.Fatalf("no command: exit code %d", code)
	}
}
