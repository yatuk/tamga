package patterns

import (
	"encoding/json"
	"errors"
	"testing"
)

// fakePersister is backing storage that can be told to fail.
type fakePersister struct {
	docs map[string][]byte
	fail error
}

func (f *fakePersister) Put(id string, doc any) error {
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

func (f *fakePersister) Delete(id string) error {
	if f.fail != nil {
		return f.fail
	}
	delete(f.docs, id)
	return nil
}

func (f *fakePersister) All(each func(id string, raw []byte) error) error {
	if f.fail != nil {
		return f.fail
	}
	for id, raw := range f.docs {
		if err := each(id, raw); err != nil {
			return err
		}
	}
	return nil
}

// A change that could not be stored must not look as if it happened: the
// next reload would silently undo it.
func TestPersist_FailedWriteChangesNothing(t *testing.T) {
	back := &fakePersister{docs: map[string][]byte{}}
	s := NewMemoryStore()
	if err := s.Persist(back); err != nil {
		t.Fatal(err)
	}
	p, err := s.Create(Pattern{Name: "a", Kind: KindLiteral, Pattern: "x", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	back.fail = errors.New("database is down")
	if _, err := s.Create(Pattern{Name: "b", Kind: KindLiteral, Pattern: "y"}); err == nil {
		t.Fatal("create: want the storage error")
	}
	if _, err := s.Update(p.ID, Pattern{Name: "a2", Kind: KindLiteral, Pattern: "x"}); err == nil {
		t.Fatal("update: want the storage error")
	}
	if err := s.Delete(p.ID); err == nil {
		t.Fatal("delete: want the storage error")
	}
	// A reload that fails keeps the working copy.
	if err := s.Reload(); err == nil {
		t.Fatal("reload: want the storage error")
	}
	list := s.List()
	if len(list) != 1 || list[0].Name != "a" {
		t.Fatalf("working copy changed: %+v", list)
	}
}
