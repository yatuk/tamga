package store

import (
	"encoding/json"
	"errors"
	"sort"
	"testing"
)

// ssoDocs is backing storage that can be told to fail. Two stores over one
// ssoDocs are what a restart, or a second replica, looks like.
type ssoDocs struct {
	docs map[string][]byte
	fail error
}

func (f *ssoDocs) Put(id string, doc any) error {
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

func (f *ssoDocs) Delete(id string) error {
	if f.fail != nil {
		return f.fail
	}
	delete(f.docs, id)
	return nil
}

func (f *ssoDocs) All(each func(id string, raw []byte) error) error {
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

func TestSSOSettings_SurviveARestart(t *testing.T) {
	back := &ssoDocs{docs: map[string][]byte{}}
	a := NewMemorySSOSettingsStore()
	if err := a.Persist(back); err != nil {
		t.Fatal(err)
	}
	want := SSOSettings{
		ProviderType:     "saml",
		MetadataURL:      "https://idp.example.com/metadata",
		AttributeMapping: map[string]string{"email": "mail", "name": "cn"},
		Enabled:          true,
		Domain:           "example.com",
	}
	if err := a.Set("org-1", want); err != nil {
		t.Fatal(err)
	}

	// The restart: a new store over the same documents.
	b := NewMemorySSOSettingsStore()
	if err := b.Persist(back); err != nil {
		t.Fatal(err)
	}
	got := b.Get("org-1")
	if got.ProviderType != want.ProviderType || got.MetadataURL != want.MetadataURL || got.Domain != want.Domain || !got.Enabled ||
		got.AttributeMapping["email"] != "mail" || got.AttributeMapping["name"] != "cn" {
		t.Fatalf("after restart: %+v", got)
	}
	// An organisation nothing was stored for still gets the defaults.
	if other := b.Get("org-2"); other.Enabled || other.AttributeMapping["email"] != "email" {
		t.Fatalf("org-2: %+v", other)
	}
}

func TestSSOSettings_SecondReplicaPicksUpTheChangeOnReload(t *testing.T) {
	back := &ssoDocs{docs: map[string][]byte{}}
	a, b := NewMemorySSOSettingsStore(), NewMemorySSOSettingsStore()
	for _, s := range []*MemorySSOSettingsStore{a, b} {
		if err := s.Persist(back); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Set("org-1", SSOSettings{ProviderType: "oidc", MetadataURL: "https://old.example.com", Domain: "example.com", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if b.Get("org-1").Enabled {
		t.Fatal("the other replica has the change before it reloaded")
	}
	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := b.Get("org-1"); !got.Enabled || got.MetadataURL != "https://old.example.com" {
		t.Fatalf("after reload: %+v", got)
	}

	// Turning SSO off on one replica turns it off on the other too.
	if err := a.Set("org-1", SSOSettings{ProviderType: "oidc", MetadataURL: "https://new.example.com", Domain: "example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := b.Get("org-1"); got.Enabled || got.MetadataURL != "https://new.example.com" {
		t.Fatalf("after the second reload: %+v", got)
	}
}

// A change that could not be stored must not look as if it happened: the
// next reload would silently undo it.
func TestSSOSettings_FailedWriteChangesNothing(t *testing.T) {
	back := &ssoDocs{docs: map[string][]byte{}}
	s := NewMemorySSOSettingsStore()
	if err := s.Persist(back); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("org-1", SSOSettings{ProviderType: "saml", Domain: "example.com"}); err != nil {
		t.Fatal(err)
	}

	back.fail = errors.New("database is down")
	if err := s.Set("org-1", SSOSettings{ProviderType: "oidc", Domain: "evil.example", Enabled: true}); err == nil {
		t.Fatal("set: want the storage error")
	}
	// A reload that fails keeps the working copy.
	if err := s.Reload(); err == nil {
		t.Fatal("reload: want the storage error")
	}
	if got := s.Get("org-1"); got.ProviderType != "saml" || got.Domain != "example.com" || got.Enabled {
		t.Fatalf("working copy changed: %+v", got)
	}
}

// The stored copy is the caller's value at the time of the call.
func TestSSOSettings_SetCopiesTheMapping(t *testing.T) {
	s := NewMemorySSOSettingsStore()
	mapping := map[string]string{"email": "mail"}
	if err := s.Set("org-1", SSOSettings{AttributeMapping: mapping}); err != nil {
		t.Fatal(err)
	}
	mapping["email"] = "changed-afterwards"
	if got := s.Get("org-1").AttributeMapping["email"]; got != "mail" {
		t.Fatalf("the stored mapping followed the caller's map: %q", got)
	}
}

// The record is stored unencrypted and returned whole by GET /settings/sso
// because none of its fields is a secret. A new field has to be looked at
// before it joins them: this fails until someone has decided.
func TestSSOSettings_StoredDocumentHasOnlyTheKnownFields(t *testing.T) {
	back := &ssoDocs{docs: map[string][]byte{}}
	s := NewMemorySSOSettingsStore()
	if err := s.Persist(back); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("org-1", SSOSettings{ProviderType: "saml", MetadataURL: "https://idp.example.com", Domain: "example.com", Enabled: true, AttributeMapping: map[string]string{"email": "email"}}); err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(back.docs["org-1"], &doc); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(doc))
	for k := range doc {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"attribute_mapping", "domain", "enabled", "metadata_url", "provider_type"}
	if len(got) != len(want) {
		t.Fatalf("stored fields = %v, want %v: if the new field is a secret, the sso_settings documents must be encrypted and the field kept out of the API response", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stored fields = %v, want %v: if the new field is a secret, the sso_settings documents must be encrypted and the field kept out of the API response", got, want)
		}
	}
}
