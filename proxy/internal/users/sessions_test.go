package users

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeTable is backing storage two stores can share, as two replicas share
// the database.
type fakeTable struct {
	mu   sync.Mutex
	docs map[string][]byte
	fail error
}

func newFakeTable() *fakeTable { return &fakeTable{docs: map[string][]byte{}} }

func (f *fakeTable) Put(id string, doc any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	f.docs[id] = raw
	return nil
}

func (f *fakeTable) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.docs, id)
	return nil
}

func (f *fakeTable) All(each func(id string, raw []byte) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, raw := range f.docs {
		if err := each(id, raw); err != nil {
			return err
		}
	}
	return nil
}

func TestRoleChangeRevokesSessions(t *testing.T) {
	s := NewMemoryStore()
	if got := s.SessionsValidFrom("u1"); got != 0 {
		t.Fatalf("nothing revoked yet, got %d", got)
	}
	before := time.Now().Unix()
	if _, err := s.Set("u1", RoleAnalyst); err != nil {
		t.Fatal(err)
	}
	first := s.SessionsValidFrom("u1")
	if first < before {
		t.Fatalf("assigning a role: valid from %d, want >= %d", first, before)
	}
	// The same role again changes nothing a token was issued for.
	if _, err := s.Set("u1", RoleAnalyst); err != nil {
		t.Fatal(err)
	}
	if got := s.SessionsValidFrom("u1"); got != first {
		t.Errorf("same role again moved the revocation from %d to %d", first, got)
	}
	// Each change moves it forward, within one second too.
	if _, err := s.Set("u1", RoleViewer); err != nil {
		t.Fatal(err)
	}
	second := s.SessionsValidFrom("u1")
	if second <= first {
		t.Errorf("demotion: valid from %d, want after %d", second, first)
	}
	third, err := s.RevokeSessions("u1")
	if err != nil {
		t.Fatal(err)
	}
	if third <= second || s.SessionsValidFrom("u1") != third {
		t.Errorf("explicit revocation: %d, want after %d", third, second)
	}
	if role, _ := s.Role("u1"); role != RoleViewer {
		t.Errorf("a revocation changed the role to %q", role)
	}
}

// A removed role leaves the revocation behind: assigning a role again must
// not bring a token from before back to life.
func TestRemoveKeepsTheRevocation(t *testing.T) {
	s := NewMemoryStore()
	_, _ = s.Set("admin-1", RoleAdmin)
	_, _ = s.Set("u1", RoleAnalyst)
	if err := s.Remove("u1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Role("u1"); ok {
		t.Fatal("the role is still there")
	}
	removedAt := s.SessionsValidFrom("u1")
	if removedAt == 0 {
		t.Fatal("removing the role revoked nothing")
	}
	// It is listed as an account that lost its role, not as a member.
	listed := func() (Member, bool) {
		for _, m := range s.List() {
			if m.UserID == "u1" {
				return m, true
			}
		}
		return Member{}, false
	}
	if m, ok := listed(); !ok || m.Role != "" || m.Assigned || !m.RoleRemoved || m.SessionsValidFrom != removedAt {
		t.Fatalf("the removed account in the list = %+v, %v", m, ok)
	}
	for _, m := range s.List() {
		if m.UserID == "admin-1" && (!m.Assigned || m.RoleRemoved) {
			t.Errorf("a member with a role = %+v", m)
		}
	}
	// Not for ever: the record stays, the list forgets it.
	s.mu.Lock()
	old := s.data["u1"]
	old.UpdatedAt = time.Now().Add(-removedListedFor - time.Minute)
	s.data["u1"] = old
	s.mu.Unlock()
	if _, ok := listed(); ok {
		t.Error("an account removed long ago is still listed")
	}
	if s.SessionsValidFrom("u1") != removedAt {
		t.Error("the list forgetting the account lost its revocation")
	}
	if err := s.Remove("u1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing it twice: %v, want ErrNotFound", err)
	}
	if _, err := s.Set("u1", RoleViewer); err != nil {
		t.Fatal(err)
	}
	if got := s.SessionsValidFrom("u1"); got <= removedAt {
		t.Errorf("after assigning again: valid from %d, want after %d", got, removedAt)
	}
	if m, ok := listed(); !ok || m.Role != RoleViewer || !m.Assigned || m.RoleRemoved {
		t.Errorf("after assigning again, in the list = %+v, %v", m, ok)
	}
	// Delete, the older name for the same thing, keeps it too.
	s.Delete("u1")
	if _, ok := s.Role("u1"); ok || s.SessionsValidFrom("u1") == 0 {
		t.Error("Delete lost the role's revocation or kept the role")
	}
}

// A user without a role can be signed out: that is an account admitted by
// the operator's default role.
func TestRevokeSessionsOfAnUnassignedUser(t *testing.T) {
	s := NewMemoryStore()
	from, err := s.RevokeSessions("stranger")
	if err != nil || from == 0 {
		t.Fatalf("RevokeSessions = %d, %v", from, err)
	}
	if _, ok := s.Role("stranger"); ok {
		t.Error("revoking sessions gave the user a role")
	}
	if len(s.List()) != 0 {
		t.Error("revoking sessions made the user a member")
	}
}

// What one replica revokes, another refuses after its next reload, and a
// restart does not forget.
func TestRevocationReachesOtherReplicas(t *testing.T) {
	table := newFakeTable()
	a, b := NewMemoryStore(), NewMemoryStore()
	if err := a.Persist(table); err != nil {
		t.Fatal(err)
	}
	if err := b.Persist(table); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Set("u1", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	from, err := a.RevokeSessions("u1")
	if err != nil {
		t.Fatal(err)
	}
	// Until b reloads it answers with what it last read.
	if got := b.SessionsValidFrom("u1"); got != 0 {
		t.Fatalf("b knew of the revocation before reloading: %d", got)
	}
	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := b.SessionsValidFrom("u1"); got != from {
		t.Errorf("b after reload: %d, want %d", got, from)
	}
	restarted := NewMemoryStore()
	if err := restarted.Persist(table); err != nil {
		t.Fatal(err)
	}
	if got := restarted.SessionsValidFrom("u1"); got != from {
		t.Errorf("after a restart: %d, want %d", got, from)
	}
	if role, _ := restarted.Role("u1"); role != RoleAdmin {
		t.Errorf("after a restart the role is %q", role)
	}
	// So is what the list says of an account: that it was given a role, or
	// that the role was removed.
	_, _ = a.Set("u2", RoleViewer)
	_, _ = a.Set("u3", RoleViewer)
	if err := a.Remove("u3"); err != nil {
		t.Fatal(err)
	}
	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}
	seen := map[string]Member{}
	for _, m := range b.List() {
		seen[m.UserID] = m
	}
	if m := seen["u2"]; !m.Assigned || m.RoleRemoved {
		t.Errorf("an assigned account after reload = %+v", m)
	}
	if m, ok := seen["u3"]; !ok || m.Assigned || !m.RoleRemoved || m.Role != "" || m.SessionsValidFrom == 0 {
		t.Errorf("a removed account after reload = %+v, %v", m, ok)
	}

	// A write that fails changes nothing here either.
	table.fail = errors.New("database down")
	if _, err := a.RevokeSessions("u1"); err == nil {
		t.Fatal("a revocation that was not stored was reported as done")
	}
	if got := a.SessionsValidFrom("u1"); got != from {
		t.Errorf("a failed write moved the revocation to %d", got)
	}
}

// A replica that has not reloaded yet makes its change to what the storage
// holds, not to its own older copy: it does not write a revocation away,
// and it does not count an admin another replica has demoted.
func TestChangeOnAStaleReplica(t *testing.T) {
	table := newFakeTable()
	a, b := NewMemoryStore(), NewMemoryStore()
	_ = a.Persist(table)
	_, _ = a.Set("u1", RoleAnalyst)
	_, _ = a.Set("a1", RoleAdmin)
	_, _ = a.Set("a2", RoleAdmin)
	_ = b.Persist(table)

	from, err := a.RevokeSessions("u1")
	if err != nil {
		t.Fatal(err)
	}
	// b still holds the record from before the revocation and writes it.
	if _, err := b.Set("u1", RoleAnalyst); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := a.SessionsValidFrom("u1"); got < from {
		t.Errorf("a write from a stale replica moved the revocation back from %d to %d", from, got)
	}

	// a demotes one admin; b, which has not reloaded, is asked to demote
	// the other.
	if _, err := a.Assign("a1", RoleViewer); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Assign("a2", RoleViewer); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("the stale replica demoted the last admin: %v", err)
	}
}

func TestAssignKeepsTheLastAdmin(t *testing.T) {
	s := NewMemoryStore()
	_, _ = s.Set("a1", RoleAdmin)
	if _, err := s.Assign("a1", RoleViewer); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the only admin: %v, want ErrLastAdmin", err)
	}
	if err := s.Remove("a1"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("removing the only admin: %v, want ErrLastAdmin", err)
	}
	if _, err := s.Assign("a1", RoleAdmin); err != nil {
		t.Fatalf("keeping the only admin an admin: %v", err)
	}
	_, _ = s.Set("a2", RoleAdmin)
	if _, err := s.Assign("a1", RoleViewer); err != nil {
		t.Fatalf("demoting one of two admins: %v", err)
	}
	if err := s.Remove("a2"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("removing the admin that is left: %v, want ErrLastAdmin", err)
	}
}

// Two admins, each demoted at the same moment by a different request: one
// goes through and one is refused, every time. Without the check and the
// write being one step, both find the other still an admin.
func TestConcurrentDemotionsLeaveAnAdmin(t *testing.T) {
	for round := 0; round < 300; round++ {
		s := NewMemoryStore()
		if round%2 == 1 {
			// With backing storage the check reads it first.
			if err := s.Persist(newFakeTable()); err != nil {
				t.Fatal(err)
			}
		}
		_, _ = s.Set("a1", RoleAdmin)
		_, _ = s.Set("a2", RoleAdmin)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		for i, id := range []string{"a1", "a2"} {
			wg.Add(1)
			go func(i int, id string) {
				defer wg.Done()
				<-start
				if i == 0 {
					_, errs[i] = s.Assign(id, RoleViewer)
				} else {
					errs[i] = s.Remove(id)
				}
			}(i, id)
		}
		close(start)
		wg.Wait()
		admins := 0
		for _, m := range s.List() {
			if m.Role == RoleAdmin {
				admins++
			}
		}
		refused := 0
		for _, err := range errs {
			if errors.Is(err, ErrLastAdmin) {
				refused++
			} else if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		if admins != 1 || refused != 1 {
			t.Fatalf("round %d: %d admins left, %d demotions refused; want 1 and 1", round, admins, refused)
		}
	}
}
