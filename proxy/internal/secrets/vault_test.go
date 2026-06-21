package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestVaultProvider_Resolve(t *testing.T) {
	token := "test-token"

	// Mock Vault server that returns KV v2 responses.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header.
		if r.Header.Get("X-Vault-Token") != token {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []string{"permission denied"}})
			return
		}

		// KV v2 read: GET /v1/secret/data/{key}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/secret/data/tamga/admin_key":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"value": "admin-secret-123",
					},
					"metadata": map[string]interface{}{
						"version": 3,
					},
				},
			})
		case "/v1/secret/data/tamga/empty_key":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{},
					"metadata": map[string]interface{}{
						"version": 1,
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []string{}})
		}
	}))
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Addr:       srv.URL,
		Token:      token,
		SecretPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider: %v", err)
	}
	defer vp.Close()

	ctx := context.Background()

	// Successful resolve.
	val, err := vp.Resolve(ctx, "tamga/admin_key")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "admin-secret-123" {
		t.Errorf("expected admin-secret-123, got %q", val)
	}

	// Missing value field.
	_, err = vp.Resolve(ctx, "tamga/empty_key")
	if err == nil {
		t.Error("expected error for missing value field")
	}

	// Non-existent key.
	_, err = vp.Resolve(ctx, "tamga/nonexistent")
	if err == nil {
		t.Error("expected error for non-existent key")
	}
}

func TestVaultProvider_ResolveBatch(t *testing.T) {
	token := "batch-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/secret/data/key_a":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{"value": "val_a"},
				},
			})
		case "/v1/secret/data/key_b":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{"value": "val_b"},
				},
			})
		case "/v1/secret/data/key_c":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Addr:       srv.URL,
		Token:      token,
		SecretPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider: %v", err)
	}
	defer vp.Close()

	ctx := context.Background()
	result, err := vp.ResolveBatch(ctx, []string{"key_a", "key_b", "key_c"})
	if err == nil {
		t.Error("expected error for partial batch failure")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
	if result["key_a"] != "val_a" {
		t.Errorf("expected val_a, got %q", result["key_a"])
	}
	if result["key_b"] != "val_b" {
		t.Errorf("expected val_b, got %q", result["key_b"])
	}
	if _, ok := result["key_c"]; ok {
		t.Error("expected key_c to be absent")
	}
}

func TestVaultProvider_HealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"initialized": true,
			"sealed":      false,
			"version":     "1.15.0",
		})
	}))
	defer srv.Close()

	// Without token — health check uses /v1/sys/health.
	vp, err := NewVaultProvider(VaultConfig{
		Addr:       srv.URL,
		SecretPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider: %v", err)
	}
	defer vp.Close()

	ctx := context.Background()
	if err := vp.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck: %v", err)
	}
}

func TestVaultProvider_HealthCheck_WithToken(t *testing.T) {
	token := "health-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/v1/auth/token/lookup-self" {
			if r.Header.Get("X-Vault-Token") != token {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"policies": []string{"default", "tamga"},
				},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"initialized": true,
			"sealed":      false,
		})
	}))
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Addr:       srv.URL,
		Token:      token,
		SecretPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider: %v", err)
	}
	defer vp.Close()

	ctx := context.Background()
	if err := vp.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck with token: %v", err)
	}
}

func TestVaultProvider_HealthCheck_Down(t *testing.T) {
	// Test with a server that returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	url := srv.URL
	srv.Close() // close it so connection fails

	vp, err := NewVaultProvider(VaultConfig{
		Addr:       url,
		SecretPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider: %v", err)
	}
	defer vp.Close()

	ctx := context.Background()
	err = vp.HealthCheck(ctx)
	if err == nil {
		t.Error("expected health check error for down server")
	}
}

func TestVaultProvider_Enabled(t *testing.T) {
	vp, err := NewVaultProvider(VaultConfig{
		Addr:       "http://localhost:8200",
		SecretPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider: %v", err)
	}
	defer vp.Close()

	if !vp.Enabled() {
		t.Error("expected VaultProvider to be enabled")
	}
}

func TestVaultProvider_TokenFile(t *testing.T) {
	// Test that token file is read correctly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "file-token-content" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{"value": "ok"},
			},
		})
	}))
	defer srv.Close()

	// Create a temp token file.
	tmpFile := t.TempDir() + "/vault-token"
	if err := os.WriteFile(tmpFile, []byte("file-token-content"), 0644); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	vp, err := NewVaultProvider(VaultConfig{
		Addr:       srv.URL,
		TokenFile:  tmpFile,
		SecretPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider: %v", err)
	}
	defer vp.Close()

	// Token from file should override empty Token field.
	val, err := vp.Resolve(context.Background(), "tamga/admin_key")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "ok" {
		t.Errorf("expected ok, got %q", val)
	}
}

func TestVaultProvider_MissingAddr(t *testing.T) {
	_, err := NewVaultProvider(VaultConfig{
		Addr: "",
	})
	if err == nil {
		t.Error("expected error for missing address")
	}
}
