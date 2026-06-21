package api

import (
	"net/http"

	"github.com/rs/zerolog/log"
)

// These thin stubs keep the router compiling while their corresponding
// Sprint 2/3 phases fill in the real logic. Every handler writes an
// informative JSON body so callers see a sane 501 rather than a crash.

func (cfg Config) handleAuditVerify(w http.ResponseWriter, r *http.Request) {
	if cfg.Audit == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"chain_ok": true, "entries": 0, "note": "in-memory ring"})
		return
	}
	// The hash chain verification implementation lives in audit_chain.go and
	// delegates to the persistent audit store.
	result := verifyAuditChain(cfg.Audit)
	writeJSON(w, http.StatusOK, result)
}

func (cfg Config) handleSubjectErase(w http.ResponseWriter, r *http.Request) {
	// KVKK/GDPR subject erase; implementation in privacy.go.
	handleSubjectEraseImpl(cfg, w, r)
}

func (cfg Config) handleSubjectAccess(w http.ResponseWriter, r *http.Request) {
	// GDPR Art. 15 / KVKK madde 11 subject access; implementation in privacy.go.
	handleSubjectAccessImpl(cfg, w, r)
}

func (cfg Config) handleBudgetStats(w http.ResponseWriter, r *http.Request) {
	// Budget enforcement metrics; implementation in budget.go.
	handleBudgetStatsImpl(cfg, w, r)
}

func (cfg Config) handleProvidersList(w http.ResponseWriter, r *http.Request) {
	// When the pricing store is wired, build the provider catalog from
	// active DB rows. Otherwise fall back to the hardcoded catalog.
	if cfg.PricingStore != nil {
		pricing, err := cfg.PricingStore.ListActive(r.Context())
		if err == nil && len(pricing) > 0 {
			writeJSON(w, http.StatusOK, providerCatalogDB(pricing))
			return
		} else if err != nil {
			log.Warn().Err(err).Msg("pricing store lookup failed, falling back to hardcoded catalog")
		}
	}
	writeJSON(w, http.StatusOK, providerCatalog())
}
