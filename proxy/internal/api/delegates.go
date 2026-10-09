package api

import (
	"context"
	"net/http"
	"time"

	"github.com/yatuk/tamga/internal/pricing"
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
	prices := cfg.activePricing(r.Context())
	// The models seen: from request_logs over the last 30 days when there is
	// a database, from the recent buffer otherwise.
	if h := cfg.history(); h != nil {
		now := time.Now().UTC()
		ctx, cancel := context.WithTimeout(r.Context(), historyTimeout)
		rows, err := h.ModelCounts(ctx, cfg.DefaultOrgID, now.Add(-catalogWindow), now)
		cancel()
		if err == nil {
			seen := map[string]map[string]int64{}
			for _, row := range rows {
				if row.Model == "" {
					continue
				}
				prov := pricing.CanonicalProvider(row.Provider)
				if seen[prov] == nil {
					seen[prov] = map[string]int64{}
				}
				seen[prov][row.Model] += row.Requests
			}
			writeSourced(w, sourceDatabase, providerCatalogOf(seen, prices))
			return
		}
		historyUnavailable("providers", err)
	}
	writeSourced(w, sourceMemory, providerCatalog(cfg.Recent, prices))
}
