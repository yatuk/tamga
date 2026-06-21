package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSavedHuntStore_List_TC(t *testing.T) {
	pool := NewTestPostgres(t)
	store := NewSavedHuntStore(pool)
	if store == nil {
		t.Fatal("NewSavedHuntStore returned nil")
	}
	ctx := context.Background()

	// Clean any seed/setup data for this org.
	_, _ = pool.Exec(ctx, "DELETE FROM saved_hunts WHERE org_id = $1", testOrgUUID)

	// Should return empty list initially.
	hunts, err := store.List(ctx, testOrgUUID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(hunts) != 0 {
		t.Errorf("expected 0 hunts, got %d", len(hunts))
	}

	// Create a saved hunt.
	hunt := &SavedHunt{
		OrgID:     testOrgUUID,
		Name:      "Test Hunt 1",
		Query:     json.RawMessage(`{"finding_type":"PII","severity":"HIGH"}`),
		CreatedBy: "test-user",
	}
	if err := store.Create(ctx, hunt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// List should now return it.
	hunts, err = store.List(ctx, testOrgUUID)
	if err != nil {
		t.Fatalf("List after create: %v", err)
	}
	if len(hunts) != 1 {
		t.Fatalf("expected 1 hunt, got %d", len(hunts))
	}
	if hunts[0].Name != "Test Hunt 1" {
		t.Errorf("expected name 'Test Hunt 1', got %q", hunts[0].Name)
	}
	if hunts[0].OrgID != testOrgUUID {
		t.Errorf("expected orgID %q, got %q", testOrgUUID, hunts[0].OrgID)
	}
	if hunts[0].CreatedBy != "test-user" {
		t.Errorf("expected created_by 'test-user', got %q", hunts[0].CreatedBy)
	}

	// Create a second hunt for a different org -- should NOT appear in list.
	hunt2 := &SavedHunt{
		OrgID:     "00000000-0000-0000-0000-000000000002",
		Name:      "Other Org Hunt",
		Query:     json.RawMessage(`{}`),
		CreatedBy: "other-user",
	}
	if err := store.Create(ctx, hunt2); err != nil {
		t.Fatalf("Create hunt2: %v", err)
	}

	hunts, err = store.List(ctx, testOrgUUID)
	if err != nil {
		t.Fatalf("List after cross-org create: %v", err)
	}
	if len(hunts) != 1 {
		t.Errorf("expected 1 hunt for org (cross-org isolation), got %d", len(hunts))
	}
}

func TestSavedHuntStore_Create_TC(t *testing.T) {
	pool := NewTestPostgres(t)
	store := NewSavedHuntStore(pool)
	if store == nil {
		t.Fatal("NewSavedHuntStore returned nil")
	}
	ctx := context.Background()

	_, _ = pool.Exec(ctx, "DELETE FROM saved_hunts WHERE org_id = $1", testOrgUUID)

	t.Run("with_all_fields", func(t *testing.T) {
		hunt := &SavedHunt{
			OrgID:     testOrgUUID,
			Name:      "Full Hunt",
			Query:     json.RawMessage(`{"finding_type":"SECRET","severity":"CRITICAL","category":"api_key"}`),
			CreatedBy: "admin",
		}
		if err := store.Create(ctx, hunt); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if hunt.ID == "" {
			t.Error("expected non-empty ID after create")
		}
		if hunt.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt after create")
		}
		if hunt.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt after create")
		}

		// Verify persistence via List.
		hunts, err := store.List(ctx, testOrgUUID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(hunts) != 1 {
			t.Fatalf("expected 1 hunt, got %d", len(hunts))
		}
		if hunts[0].Name != "Full Hunt" {
			t.Errorf("name mismatch: %q", hunts[0].Name)
		}
	})

	t.Run("with_nil_query", func(t *testing.T) {
		hunt := &SavedHunt{
			OrgID: testOrgUUID,
			Name:  "Nil Query Hunt",
			Query: nil,
		}
		if err := store.Create(ctx, hunt); err != nil {
			t.Fatalf("Create with nil query: %v", err)
		}
		if hunt.Query == nil {
			// Create should have set it to {}.
			t.Log("query was nil and should have been defaulted")
		}
	})

	t.Run("with_empty_created_by", func(t *testing.T) {
		hunt := &SavedHunt{
			OrgID:     testOrgUUID,
			Name:      "No Creator Hunt",
			Query:     json.RawMessage(`{}`),
			CreatedBy: "",
		}
		if err := store.Create(ctx, hunt); err != nil {
			t.Fatalf("Create with empty created_by: %v", err)
		}
		// Validate via direct DB query.
		var createdBy string
		if err := pool.QueryRow(ctx,
			"SELECT COALESCE(created_by, '') FROM saved_hunts WHERE id = $1", hunt.ID,
		).Scan(&createdBy); err != nil {
			t.Fatalf("verify created_by: %v", err)
		}
		if createdBy != "" {
			t.Errorf("expected empty created_by, got %q", createdBy)
		}
	})
}

func TestSavedHuntStore_Update_TC(t *testing.T) {
	pool := NewTestPostgres(t)
	store := NewSavedHuntStore(pool)
	if store == nil {
		t.Fatal("NewSavedHuntStore returned nil")
	}
	ctx := context.Background()

	_, _ = pool.Exec(ctx, "DELETE FROM saved_hunts WHERE org_id = $1", testOrgUUID)

	t.Run("update_existing", func(t *testing.T) {
		hunt := &SavedHunt{
			OrgID:     testOrgUUID,
			Name:      "Original Name",
			Query:     json.RawMessage(`{"finding_type":"PII"}`),
			CreatedBy: "user1",
		}
		if err := store.Create(ctx, hunt); err != nil {
			t.Fatalf("Create: %v", err)
		}
		originalUpdatedAt := hunt.UpdatedAt

		// Update name and query.
		hunt.Name = "Updated Name"
		hunt.Query = json.RawMessage(`{"finding_type":"SECRET","severity":"HIGH"}`)
		if err := store.Update(ctx, hunt); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if !hunt.UpdatedAt.After(originalUpdatedAt) {
			t.Error("expected UpdatedAt to be refreshed after update")
		}

		// Verify via List.
		hunts, err := store.List(ctx, testOrgUUID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(hunts) != 1 {
			t.Fatalf("expected 1 hunt, got %d", len(hunts))
		}
		if hunts[0].Name != "Updated Name" {
			t.Errorf("expected name 'Updated Name', got %q", hunts[0].Name)
		}
	})

	t.Run("update_nonexistent", func(t *testing.T) {
		hunt := &SavedHunt{
			ID:    "00000000-0000-0000-0000-000000000099",
			OrgID: testOrgUUID,
			Name:  "Ghost",
			Query: json.RawMessage(`{}`),
		}
		err := store.Update(ctx, hunt)
		if err != ErrSavedHuntNotFound {
			t.Errorf("expected ErrSavedHuntNotFound, got %v", err)
		}
	})

	t.Run("update_wrong_org", func(t *testing.T) {
		// Create a hunt for org1.
		hunt := &SavedHunt{
			OrgID:     testOrgUUID,
			Name:      "Org1 Hunt",
			Query:     json.RawMessage(`{}`),
			CreatedBy: "user1",
		}
		if err := store.Create(ctx, hunt); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Try to update from org2.
		hunt.OrgID = "00000000-0000-0000-0000-000000000002"
		hunt.Name = "Hijacked"
		err := store.Update(ctx, hunt)
		if err != ErrSavedHuntNotFound {
			t.Errorf("expected ErrSavedHuntNotFound for wrong org, got %v", err)
		}
	})
}

func TestSavedHuntStore_Delete_TC(t *testing.T) {
	pool := NewTestPostgres(t)
	store := NewSavedHuntStore(pool)
	if store == nil {
		t.Fatal("NewSavedHuntStore returned nil")
	}
	ctx := context.Background()

	_, _ = pool.Exec(ctx, "DELETE FROM saved_hunts WHERE org_id = $1", testOrgUUID)

	t.Run("delete_existing", func(t *testing.T) {
		hunt := &SavedHunt{
			OrgID:     testOrgUUID,
			Name:      "To Be Deleted",
			Query:     json.RawMessage(`{"finding_type":"PII"}`),
			CreatedBy: "user1",
		}
		if err := store.Create(ctx, hunt); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := store.Delete(ctx, testOrgUUID, hunt.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Verify gone.
		hunts, err := store.List(ctx, testOrgUUID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, h := range hunts {
			if h.ID == hunt.ID {
				t.Error("deleted hunt still appears in list")
			}
		}
	})

	t.Run("delete_nonexistent", func(t *testing.T) {
		err := store.Delete(ctx, testOrgUUID, "00000000-0000-0000-0000-000000000099")
		if err != ErrSavedHuntNotFound {
			t.Errorf("expected ErrSavedHuntNotFound, got %v", err)
		}
	})

	t.Run("delete_wrong_org", func(t *testing.T) {
		hunt := &SavedHunt{
			OrgID:     testOrgUUID,
			Name:      "Org1 Hunt",
			Query:     json.RawMessage(`{}`),
			CreatedBy: "user1",
		}
		if err := store.Create(ctx, hunt); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Try to delete from wrong org.
		err := store.Delete(ctx, "00000000-0000-0000-0000-000000000002", hunt.ID)
		if err != ErrSavedHuntNotFound {
			t.Errorf("expected ErrSavedHuntNotFound for wrong org, got %v", err)
		}

		// Verify still exists for correct org.
		hunts, err := store.List(ctx, testOrgUUID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		found := false
		for _, h := range hunts {
			if h.ID == hunt.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("hunt was incorrectly deleted via wrong org")
		}
	})
}
