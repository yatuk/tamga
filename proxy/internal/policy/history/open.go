package history

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// DefaultDir is where the file store keeps revisions and proposals when
// TAMGA_POLICY_HISTORY_DIR is not set.
const DefaultDir = "./data/policy-history"

// Where the policy history is kept, as Open reports it.
const (
	BackendPostgres = "postgres"
	BackendFile     = "file"
	BackendDisabled = "disabled"
)

// Open returns the store of policy revisions and proposals and says which
// one it is. With a database (hasDB) that is Postgres. When that store
// cannot be made, and when there is no database, it is the file store under
// dir; when the directory cannot be used either there is no history and the
// store is nil. Each log line says what was done, not what was meant.
func Open(hasDB bool, pool *pgxpool.Pool, dir string) (Store, string) {
	if hasDB {
		pg, err := NewPostgresStore(pool)
		if err == nil {
			log.Info().Msg("policy history backed by postgres")
			return pg, BackendPostgres
		}
		// The file store is on this replica's disk: other replicas do not
		// see what is written to it.
		log.Warn().Err(err).Msg("policy history: the postgres store could not be created, falling back to files on this replica")
	}
	if dir == "" {
		dir = DefaultDir
	}
	fs, err := NewFileStore(dir)
	if err != nil {
		log.Warn().Err(err).Str("dir", dir).Msg("policy history disabled: the directory could not be used")
		return nil, BackendDisabled
	}
	log.Info().Str("dir", dir).Msg("policy history backed by files")
	return fs, BackendFile
}
