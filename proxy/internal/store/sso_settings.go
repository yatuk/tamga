package store

import (
	"encoding/json"
	"sync"

	"github.com/yatuk/tamga/internal/docstore"
)

// SSOSettings represents the SAML/OIDC configuration for an organisation.
//
// None of these fields is a secret (the metadata URL is the identity
// provider's public document), so the record is stored and returned as it
// is. A field that is one (a client secret, a SCIM token, a signing key)
// must not simply be added here: the record would then have to be stored
// encrypted, as webhooks are, and the field kept out of the GET response.
type SSOSettings struct {
	ProviderType     string            `json:"provider_type"`
	MetadataURL      string            `json:"metadata_url"`
	AttributeMapping map[string]string `json:"attribute_mapping"`
	Enabled          bool              `json:"enabled"`
	Domain           string            `json:"domain"`
}

// SSOSettingsStore persists SSO configuration per organisation.
type SSOSettingsStore interface {
	Get(orgID string) *SSOSettings
	// Set fails when the change could not be stored, and then changes
	// nothing.
	Set(orgID string, cfg SSOSettings) error
}

// MemorySSOSettingsStore is an in-memory implementation of SSOSettingsStore.
// It is safe for concurrent use and returns a copy on reads to prevent
// accidental mutation by callers. With a Persister attached it is loaded
// from and written through to the database, one document per organisation.
type MemorySSOSettingsStore struct {
	// wmu serialises changes, so a slow write to the database never holds
	// the lock readers take.
	wmu     sync.Mutex
	mu      sync.RWMutex
	data    map[string]SSOSettings
	persist docstore.Persister
}

// Persist attaches backing storage and loads what it holds. From then on a
// change is written there before it is applied here.
func (m *MemorySSOSettingsStore) Persist(p docstore.Persister) error {
	m.wmu.Lock()
	m.persist = p
	m.wmu.Unlock()
	return m.Reload()
}

// Reload replaces the working copy with what the backing storage holds,
// which is how a change made on another replica arrives. The copy is swapped
// whole: a reader sees the settings from before or after, never a mix.
func (m *MemorySSOSettingsStore) Reload() error {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	if m.persist == nil {
		return nil
	}
	fresh := make(map[string]SSOSettings)
	err := m.persist.All(func(id string, raw []byte) error {
		var s SSOSettings
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		fresh[id] = s
		return nil
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.data = fresh
	m.mu.Unlock()
	return nil
}

// NewMemorySSOSettingsStore is used for test injection.
// It returns an empty in-memory SSO settings store.
func NewMemorySSOSettingsStore() *MemorySSOSettingsStore {
	return &MemorySSOSettingsStore{data: make(map[string]SSOSettings)}
}

// Get returns the SSO settings for orgID, or a zero-value default if
// nothing has been persisted. The returned value is a copy.
func (m *MemorySSOSettingsStore) Get(orgID string) *SSOSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v, ok := m.data[orgID]; ok {
		cp := v
		if v.AttributeMapping != nil {
			cp.AttributeMapping = make(map[string]string, len(v.AttributeMapping))
			for k, val := range v.AttributeMapping {
				cp.AttributeMapping[k] = val
			}
		}
		return &cp
	}
	return &SSOSettings{
		AttributeMapping: map[string]string{"email": "email", "name": "displayName"},
	}
}

// Set stores the SSO configuration for orgID.
func (m *MemorySSOSettingsStore) Set(orgID string, cfg SSOSettings) error {
	// The caller's map is not kept: it could change it afterwards.
	if cfg.AttributeMapping != nil {
		mapping := make(map[string]string, len(cfg.AttributeMapping))
		for k, v := range cfg.AttributeMapping {
			mapping[k] = v
		}
		cfg.AttributeMapping = mapping
	}
	m.wmu.Lock()
	defer m.wmu.Unlock()
	if m.persist != nil {
		if err := m.persist.Put(orgID, cfg); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.data[orgID] = cfg
	m.mu.Unlock()
	return nil
}

// NoopSSOSettingsStore returns default settings for every org and discards
// writes. It is used when the operator has not configured an SSO store,
// ensuring nil-pointer safety throughout the handler stack.
type NoopSSOSettingsStore struct{}

// NewNoopSSOSettingsStore returns a no-op SSO settings store.
func NewNoopSSOSettingsStore() *NoopSSOSettingsStore {
	return &NoopSSOSettingsStore{}
}

// Get returns the default SSO settings.
func (n *NoopSSOSettingsStore) Get(_ string) *SSOSettings {
	return &SSOSettings{
		AttributeMapping: map[string]string{"email": "email", "name": "displayName"},
	}
}

// Set discards the configuration (no-op).
func (n *NoopSSOSettingsStore) Set(_ string, _ SSOSettings) error { return nil }
