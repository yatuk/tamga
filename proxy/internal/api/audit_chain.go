package api

import (
	"github.com/yatuk/tamga/internal/incidents"
)

// verifyAuditChain walks the in-memory audit log and reports whether the
// hash-chain covering every entry is intact.
func verifyAuditChain(ring *incidents.AuditRing) map[string]interface{} {
	if ring == nil {
		return map[string]interface{}{"chain_ok": true, "entries": 0, "note": "audit ring disabled"}
	}
	ok, broken := ring.Verify()
	entries := ring.List(0)
	out := map[string]interface{}{
		"chain_ok": ok,
		"entries":  len(entries),
	}
	if !ok {
		out["broken_at"] = broken
	}
	return out
}
