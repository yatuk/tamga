package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPathNormalize_PassesCleanPaths(t *testing.T) {
	handler := PathNormalize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	clean := []string{
		"/health",
		"/api/v1/stats",
		"/api/v1/events/abc123",
		"/api/v1/incidents/req-456",
		"/",
		"/api/v1/billing/pricing",
	}
	for _, p := range clean {
		req := httptest.NewRequest("GET", p, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("clean path %q returned %d, want 200", p, rec.Code)
		}
	}
}

func TestPathNormalize_BlocksTraversal(t *testing.T) {
	handler := PathNormalize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	vectors := []struct {
		name string
		path string
	}{
		// Percent-encoded forward slash
		{"%2F ..", "/api/v1/..%2F..%2Fadmin"},
		{"lowercase %2f", "/api/v1/..%2f..%2fadmin"},
		// Percent-encoded backslash
		{"%5C ..", "/api/v1/..%5C..%5Cadmin"},
		// Double encoding
		{"double encode %252F", "/api/v1/..%252F..%252Fadmin"},
		{"double encode %252f", "/api/v1/..%252f..%252fadmin"},
		// Plain traversal
		{"plain ../", "/api/v1/../admin"},
		{"plain ..\\", "/api/v1/..\\..\\admin"},
		// UTF-8 overlong sequences
		{"overlong %c0%af", "/api/v1/..%c0%af..%c0%afadmin"},
		// Mixed encoding
		{"mixed %2F and ..", "/api/v1/..%2Fadmin/..%2Fsecrets"},
		// Triple encoding
		{"triple encode", "/api/v1/..%25252F..%25252Fadmin"},
		// Encoded dot-dot
		{"encoded dots", "/api/v1/%2e%2e/%2e%2e/admin"},
		{"encoded dots mixed case", "/api/v1/%2E%2E/%2e%2e/admin"},
	}

	for _, tt := range vectors {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("[%s] path %q returned %d, want 403", tt.name, tt.path, rec.Code)
			}
		})
	}
}

func TestPathNormalize_PassesTraversalFree(t *testing.T) {
	handler := PathNormalize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Paths that contain percent-encoded chars but NOT traversal.
	safe := []string{
		"/api/v1/events/request%20id",   // space
		"/api/v1/events/request%2Did",   // dash
		"/api/v1/user%40example.com",    // @ (email in path)
		"/api/v1/hello%2Fworld",         // literal / in path component
		"/api/v1/items/foo%2Fbar%2Fbaz", // multiple encoded /
	}
	for _, p := range safe {
		req := httptest.NewRequest("GET", p, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("safe path %q returned %d, want 200", p, rec.Code)
		}
	}
}

func TestPathNormalize_DecodedPathForwarded(t *testing.T) {
	// Verify that decoded paths are written back to r.URL.Path.
	var captured string
	handler := PathNormalize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/events/request%20id", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal("expected 200")
	}
	// %20 should be decoded to space.
	if captured != "/api/v1/events/request id" {
		t.Errorf("path not decoded: got %q", captured)
	}
}

func TestPathNormalize_ConsecutiveSlashes(t *testing.T) {
	handler := PathNormalize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Multiple consecutive slashes should not be treated as traversal.
	req := httptest.NewRequest("GET", "/api/v1//events//test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("consecutive slashes should pass: got %d", rec.Code)
	}
}
