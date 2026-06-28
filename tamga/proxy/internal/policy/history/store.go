// Package history stores append-only revisions of the policy file plus
// two-person "proposal" workflow metadata. The on-disk layout is a single
// JSON file so it ships in the default binary with no extra dependency.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Revision is a single immutable snapshot of tamga-policy.yaml.
type Revision struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	YAML      string    `json:"yaml"`
	ParentID  string    `json:"parent_id,omitempty"`
}

// Proposal is a pending policy change awaiting approval.
type Proposal struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Author     string    `json:"author"`
	Message    string    `json:"message"`
	YAML       string    `json:"yaml"`
	Status     string    `json:"status"` // open, approved, rejected
	ApprovedBy string    `json:"approved_by,omitempty"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
	RejectedBy string    `json:"rejected_by,omitempty"`
	RejectedAt time.Time `json:"rejected_at,omitempty"`
}

// Store is the read/write interface used by API handlers.
type Store interface {
	AppendRevision(rev Revision) (Revision, error)
	ListRevisions() ([]Revision, error)
	GetRevision(id string) (Revision, bool)
	CreateProposal(p Proposal) (Proposal, error)
	ListProposals() ([]Proposal, error)
	GetProposal(id string) (Proposal, bool)
	UpdateProposal(p Proposal) error
}

// FileStore persists revisions + proposals as two JSON files under dir.
type FileStore struct {
	mu   sync.RWMutex
	dir  string
	revs []Revision
	prop []Proposal
}

func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("history dir required")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	fs := &FileStore{dir: dir}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (s *FileStore) revisionsPath() string { return filepath.Join(s.dir, "revisions.json") }
func (s *FileStore) proposalsPath() string { return filepath.Join(s.dir, "proposals.json") }

func (s *FileStore) load() error {
	if err := readJSON(s.revisionsPath(), &s.revs); err != nil {
		return err
	}
	if err := readJSON(s.proposalsPath(), &s.prop); err != nil {
		return err
	}
	return nil
}

func (s *FileStore) flush() error {
	if err := writeJSON(s.revisionsPath(), s.revs); err != nil {
		return err
	}
	return writeJSON(s.proposalsPath(), s.prop)
}

func (s *FileStore) AppendRevision(rev Revision) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rev.ID == "" {
		rev.ID = uuid.Must(uuid.NewV7()).String()
	}
	if rev.Timestamp.IsZero() {
		rev.Timestamp = time.Now().UTC()
	}
	if len(s.revs) > 0 {
		rev.ParentID = s.revs[len(s.revs)-1].ID
	}
	s.revs = append(s.revs, rev)
	if err := s.flush(); err != nil {
		return Revision{}, err
	}
	return rev, nil
}

func (s *FileStore) ListRevisions() ([]Revision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Revision, len(s.revs))
	copy(out, s.revs)
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	return out, nil
}

func (s *FileStore) GetRevision(id string) (Revision, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.revs {
		if r.ID == id {
			return r, true
		}
	}
	return Revision{}, false
}

func (s *FileStore) CreateProposal(p Proposal) (Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = uuid.Must(uuid.NewV7()).String()
	}
	if p.Timestamp.IsZero() {
		p.Timestamp = time.Now().UTC()
	}
	if p.Status == "" {
		p.Status = "open"
	}
	s.prop = append(s.prop, p)
	if err := s.flush(); err != nil {
		return Proposal{}, err
	}
	return p, nil
}

func (s *FileStore) ListProposals() ([]Proposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Proposal, len(s.prop))
	copy(out, s.prop)
	return out, nil
}

func (s *FileStore) GetProposal(id string) (Proposal, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.prop {
		if p.ID == id {
			return p, true
		}
	}
	return Proposal{}, false
}

func (s *FileStore) UpdateProposal(p Proposal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.prop {
		if existing.ID == p.ID {
			s.prop[i] = p
			return s.flush()
		}
	}
	return fmt.Errorf("proposal not found: %s", p.ID)
}

func readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
