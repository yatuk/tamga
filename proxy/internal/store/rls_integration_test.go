package store

import (
	"context"
	"testing"
)

// No table carries row-level security once the migrations have run. The
// policy from 011 depended on a setting the proxy never sets, and the tests
// used to switch it off to work at all. What separates organisations is the
// org_id predicate of each query, which the request-history tests check with
// two organisations. If a policy comes back, it has to come with the code
// that sets the tenant, and this test is where that shows.
func TestMigrations_LeaveNoRowLevelSecurity(t *testing.T) {
	pool := NewTestPostgres(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relrowsecurity`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		t.Errorf("table %s has row-level security enabled, and nothing sets app.tenant_id", name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	var policies int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_policies WHERE schemaname = 'public'`).Scan(&policies); err != nil {
		t.Fatal(err)
	}
	if policies != 0 {
		t.Errorf("%d row-level security policies remain", policies)
	}
}
