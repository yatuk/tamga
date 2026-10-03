package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
)

// The key store against a database built from deploy/migrations, which is
// what a restart leaves behind.
func TestPG_VirtualKeys(t *testing.T) {
	skipIfNoIntegration(t)
	pool := NewTestPostgres(t)
	ctx := context.Background()

	s, err := apikeys.NewPostgresStore(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateWith(apikeys.Params{
		Label: "billing service", Scope: apikeys.ScopeProxy,
		OrgID: "acme", Role: "analyst", UserID: "svc-billing",
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("survives a restart", func(t *testing.T) {
		// A second store on the same database has no memory of the first.
		again, err := apikeys.NewPostgresStore(ctx, pool)
		if err != nil {
			t.Fatal(err)
		}
		k, ok := again.Verify(created.RawKey)
		if !ok {
			t.Fatal("the key did not verify after a restart")
		}
		if k.OrgID != "acme" || k.Role != "analyst" || k.UserID != "svc-billing" || k.Scope != apikeys.ScopeProxy {
			t.Fatalf("identity lost: %+v", k)
		}
		list := again.List()
		if len(list) != 1 || list[0].ID != created.ID || list[0].LastUsed.IsZero() {
			t.Fatalf("list after use: %+v", list)
		}
	})

	t.Run("the value is not stored", func(t *testing.T) {
		rows, err := pool.Query(ctx, `SELECT row_to_json(k)::text FROM virtual_keys k`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				t.Fatal(err)
			}
			n++
			secret := strings.TrimPrefix(created.RawKey, created.Prefix)
			if strings.Contains(row, created.RawKey) || strings.Contains(row, secret) {
				t.Fatalf("the row holds the key: %s", row)
			}
		}
		if n != 1 {
			t.Fatalf("rows = %d, want 1", n)
		}
	})

	t.Run("a wrong key does not verify", func(t *testing.T) {
		if _, ok := s.Verify(created.RawKey + "0"); ok {
			t.Fatal("verified a key that was never issued")
		}
		if _, ok := s.Verify(""); ok {
			t.Fatal("verified an empty key")
		}
	})

	t.Run("an expired key does not verify", func(t *testing.T) {
		short, err := s.CreateWith(apikeys.Params{Label: "short", Scope: apikeys.ScopeProxy, ExpiresAt: time.Now().Add(time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Verify(short.RawKey); !ok {
			t.Fatal("the key should verify before it expires")
		}
		if _, err := pool.Exec(ctx, `UPDATE virtual_keys SET expires_at = now() - interval '1 minute' WHERE id = $1`, short.ID); err != nil {
			t.Fatal(err)
		}
		fresh, _ := apikeys.NewPostgresStore(ctx, pool)
		if _, ok := fresh.Verify(short.RawKey); ok {
			t.Fatal("an expired key verified")
		}
		if _, err := s.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy, ExpiresAt: time.Now().Add(-time.Minute)}); err == nil {
			t.Fatal("created a key that is already expired")
		}
	})

	t.Run("a revoked key does not verify", func(t *testing.T) {
		if _, ok := s.Verify(created.RawKey); !ok {
			t.Fatal("precondition: the key verifies")
		}
		if err := s.Delete(created.ID); err != nil {
			t.Fatal(err)
		}
		// On the replica that revoked it, at once, cache or not.
		if _, ok := s.Verify(created.RawKey); ok {
			t.Fatal("a revoked key verified")
		}
		for _, k := range s.List() {
			if k.ID == created.ID {
				t.Fatal("a revoked key is still listed")
			}
		}
		if err := s.Delete(created.ID); err != apikeys.ErrNotFound {
			t.Fatalf("second revoke: %v, want ErrNotFound", err)
		}
		// The row stays, for the record.
		var revoked bool
		if err := pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM virtual_keys WHERE id = $1`, created.ID).Scan(&revoked); err != nil || !revoked {
			t.Fatalf("revoked row: %v %v", revoked, err)
		}
	})
}
