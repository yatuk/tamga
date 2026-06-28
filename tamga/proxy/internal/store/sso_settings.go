package store

import "sync"

// SSOSettings represents the SAML/OIDC configuration for an organisation.
type SSOSettings struct {
	ProviderType     string            `json:"provider_type"`
	MetadataURL      string            `json:"metadata_url"`
	AttributeMapping map[string]string `json:"attribute_mapping"`
	Enabled          bool              `json:"enabled"`
	Domain           string            `json:"domain"`
}

// SSOSettingsStore persists enterprise SSO configuration per organisation.
// The default implementation is in-memory; a future Postgres-backed
// implementation can be swapped in by satisfying this interface.
type SSOSettingsStore interface {
	Get(orgID string) *SSOSettings
	Set(orgID string, cfg SSOSettings)
}

// MemorySSOSettingsStore is an in-memory implementation of SSOSettingsStore.
// It is safe for concurrent use and returns a copy on reads to prevent
// accidental mutation by callers.
type MemorySSOSettingsStore struct {
	mu   sync.RWMutex
	data map[string]SSOSettings
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
func (m *MemorySSOSettingsStore) Set(orgID string, cfg SSOSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[orgID] = cfg
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
func (n *NoopSSOSettingsStore) Set(_ string, _ SSOSettings) {}
