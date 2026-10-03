package store

import (
	"context"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/docstore"
	"github.com/yatuk/tamga/internal/patterns"
	"github.com/yatuk/tamga/internal/users"
	"github.com/yatuk/tamga/internal/vault"
	"github.com/yatuk/tamga/internal/webhooks"
)

func testCipher(t *testing.T) *vault.Cipher {
	t.Helper()
	b64, err := vault.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	key, _ := vault.KeyFromBase64(b64)
	c, err := vault.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Finding B10: custom patterns, team roles and webhooks lived in memory. A
// second store on the same database is what a restart, or another replica,
// looks like.
func TestPG_SettingsSurviveARestart(t *testing.T) {
	skipIfNoIntegration(t)
	pool := NewTestPostgres(t)
	if err := docstore.Init(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	seal := testCipher(t)

	t.Run("custom patterns", func(t *testing.T) {
		table := docstore.NewTable(pool, "pattern", nil)
		a := patterns.NewMemoryStore()
		if err := a.Persist(table); err != nil {
			t.Fatal(err)
		}
		kept, err := a.Create(patterns.Pattern{Name: "project codename", Kind: patterns.KindRegex, Pattern: `(?i)project\s+falcon`, Severity: "high", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		gone, _ := a.Create(patterns.Pattern{Name: "temp", Kind: patterns.KindLiteral, Pattern: "x", Enabled: true})
		if _, err := a.Update(kept.ID, patterns.Pattern{Name: "project codename", Kind: patterns.KindRegex, Pattern: `(?i)project\s+(falcon|osprey)`, Severity: "critical", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if err := a.Delete(gone.ID); err != nil {
			t.Fatal(err)
		}

		b := patterns.NewMemoryStore()
		if err := b.Persist(table); err != nil {
			t.Fatal(err)
		}
		list := b.List()
		if len(list) != 1 || list[0].ID != kept.ID || list[0].Severity != "critical" || !strings.Contains(list[0].Pattern, "osprey") || !list[0].Enabled {
			t.Fatalf("after restart: %+v", list)
		}

		// A change on one replica reaches the other on its next reload.
		if _, err := a.Create(patterns.Pattern{Name: "second", Kind: patterns.KindLiteral, Pattern: "y", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if err := b.Reload(); err != nil {
			t.Fatal(err)
		}
		if len(b.List()) != 2 {
			t.Fatalf("after reload: %d patterns, want 2", len(b.List()))
		}
	})

	t.Run("team roles", func(t *testing.T) {
		table := docstore.NewTable(pool, "team_role", nil)
		a := users.NewMemoryStore()
		if err := a.Persist(table); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Set("user_1", users.RoleAnalyst); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Set("user_1", users.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		_, _ = a.Set("user_2", users.RoleViewer)
		a.Delete("user_2")

		b := users.NewMemoryStore()
		if err := b.Persist(table); err != nil {
			t.Fatal(err)
		}
		if role, ok := b.Role("user_1"); !ok || role != users.RoleAdmin {
			t.Fatalf("user_1 after restart: %q %v", role, ok)
		}
		if _, ok := b.Role("user_2"); ok {
			t.Fatal("a removed member came back")
		}
	})

	t.Run("webhooks are encrypted", func(t *testing.T) {
		const url = "https://hooks.slack.example/services/T000/B000/secretpath"
		const token = "pd-routing-key-123"
		table := docstore.NewTable(pool, "webhook", seal)
		a := webhooks.NewMemoryStore()
		if err := a.Persist(table); err != nil {
			t.Fatal(err)
		}
		w, err := a.Create(webhooks.Webhook{Label: "soc", Kind: webhooks.KindPagerDuty, URL: url, AuthToken: token, Enabled: true, Headers: map[string]string{"X-Team": "soc"}})
		if err != nil {
			t.Fatal(err)
		}

		var doc string
		var encrypted bool
		if err := pool.QueryRow(context.Background(), `SELECT doc, encrypted FROM stored_documents WHERE kind = 'webhook' AND id = $1`, w.ID).Scan(&doc, &encrypted); err != nil {
			t.Fatal(err)
		}
		if !encrypted || strings.Contains(doc, "secretpath") || strings.Contains(doc, token) || strings.Contains(doc, "slack") {
			t.Fatalf("the stored webhook is readable: encrypted=%v doc=%s", encrypted, doc)
		}

		b := webhooks.NewMemoryStore()
		if err := b.Persist(table); err != nil {
			t.Fatal(err)
		}
		got, err := b.Get(w.ID)
		if err != nil || got.URL != url || got.AuthToken != token || got.Headers["X-Team"] != "soc" || !got.Enabled {
			t.Fatalf("after restart: %+v %v", got, err)
		}

		// Without the key the documents cannot be read, and that is an
		// error, not an empty list.
		if err := webhooks.NewMemoryStore().Persist(docstore.NewTable(pool, "webhook", nil)); err == nil {
			t.Fatal("loading encrypted webhooks without a key must fail")
		}
		if err := webhooks.NewMemoryStore().Persist(docstore.NewTable(pool, "webhook", testCipher(t))); err == nil {
			t.Fatal("loading encrypted webhooks with a different key must fail")
		}
	})
}
