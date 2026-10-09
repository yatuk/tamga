package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"gopkg.in/yaml.v3"
)

var errEmptyPolicyYAML = errors.New("empty policy")

// readPolicyYAMLFromRequest reads up to 256KiB: raw YAML or JSON { "yaml": "..." }.
func readPolicyYAMLFromRequest(r *http.Request) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		return "", err
	}
	yamlText := string(raw)
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		var body struct {
			YAML string `json:"yaml"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return "", err
		}
		yamlText = body.YAML
	}
	if strings.TrimSpace(yamlText) == "" {
		return "", errEmptyPolicyYAML
	}
	return yamlText, nil
}

func writePolicyValidationFailed(w http.ResponseWriter, issues []policy.ValidationIssue) {
	details := make([]map[string]interface{}, 0)
	for _, i := range issues {
		if i.Severity != "error" {
			continue
		}
		details = append(details, map[string]interface{}{
			"field":    i.Field,
			"rule":     i.Rule,
			"message":  i.Message,
			"severity": i.Severity,
		})
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    "validation_failed",
			"message": "Policy validation failed",
			"details": details,
		},
	})
}

// handlePolicyValidate checks policy YAML/JSON without writing disk (dry-run).
// Body: same as PUT /policies — raw YAML or JSON { "yaml": "..." }.
func (cfg Config) handlePolicyValidate(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	yamlText, err := readPolicyYAMLFromRequest(r)
	if err != nil {
		if errors.Is(err, errEmptyPolicyYAML) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty policy"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	pol, err := policy.LoadFromBytes([]byte(yamlText))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy parse: " + err.Error()})
		return
	}
	issues := policy.ValidateSemantics(pol)
	if policy.HasValidationErrors(issues) {
		writePolicyValidationFailed(w, issues)
		return
	}
	issues = append(issues, policy.CoverageGaps(pol)...)
	warnings := make([]map[string]interface{}, 0)
	for _, i := range issues {
		if i.Severity != "warning" {
			continue
		}
		warnings = append(warnings, map[string]interface{}{
			"field":    i.Field,
			"rule":     i.Rule,
			"message":  i.Message,
			"severity": i.Severity,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":    true,
		"warnings": warnings,
	})
}

// handlePolicyPut overwrites the on-disk policy YAML and reloads the store.
//
// Body: raw YAML (text/yaml) OR JSON { "yaml": "..." }
func (cfg Config) handlePolicyPut(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy store unavailable"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	yamlText, err := readPolicyYAMLFromRequest(r)
	if err != nil {
		if errors.Is(err, errEmptyPolicyYAML) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty policy"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	pol, err := policy.LoadFromBytes([]byte(yamlText))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy parse: " + err.Error()})
		return
	}
	if sem := policy.ValidateSemantics(pol); policy.HasValidationErrors(sem) {
		writePolicyValidationFailed(w, sem)
		return
	}
	if err := writePolicyFile(cfg.PolicyPath, []byte(yamlText)); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload: " + err.Error()})
		return
	}
	// Refresh policy-driven scanners after write+reload.
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.CompetitorScanner != nil {
		cfg.CompetitorScanner.Refresh()
	}
	pol = cfg.PolicyStore.GetPolicy()
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "policy.put",
			Target: pol.Name,
			Detail: map[string]interface{}{"version": pol.Version, "size": len(yamlText)},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "name": pol.Name, "version": pol.Version})
}

// A simulation takes a policy of up to 256 KiB, as PUT /policies does, and a
// sample text of up to 256 KiB, as POST /api/v1/scan does. The built-in
// scanners need about a second for that much text on one core.
const (
	maxSimulateBodyBytes = 512 << 10
	maxSimulateTextBytes = 256 << 10
)

// maxEntityBodyBytes bounds the body of a custom entity create.
const maxEntityBodyBytes = 16 << 10

// handlePolicySimulate runs the full scan + policy pipeline on user-supplied
// YAML and sample text without writing anything to disk.
//
// The policy is the caller's, and so are its regular expressions: it is
// loaded like any other, so a pattern past the limits is refused, and the
// pattern scanners stop at the request's deadline. A match is returned
// masked, as it would be stored.
//
// Body: { "yaml": "...", "sample_text": "..." }
func (cfg Config) handlePolicySimulate(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var body struct {
		YAML       string `json:"yaml"`
		SampleText string `json:"sample_text"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSimulateBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body exceeds 524288 bytes"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(body.SampleText) > maxSimulateTextBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "sample_text exceeds 262144 bytes"})
		return
	}

	// If no YAML supplied, simulate against the active policy.
	var pol *policy.Policy
	if strings.TrimSpace(body.YAML) == "" {
		if cfg.PolicyStore == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no policy to simulate"})
			return
		}
		// Refresh policy-driven scanners after reload.
		if cfg.CustomScanner != nil {
			cfg.CustomScanner.Refresh()
		}
		if cfg.CompetitorScanner != nil {
			cfg.CompetitorScanner.Refresh()
		}
		pol = cfg.PolicyStore.GetPolicy()
	} else {
		p, err := policy.LoadFromBytes([]byte(body.YAML))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy parse: " + err.Error()})
			return
		}
		pol = p
	}
	if pol == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy unavailable"})
		return
	}

	registry := scanner.NewRegistry()
	registry.Register(scanner.NewPIIScanner())
	registry.Register(scanner.NewSecretScanner())
	registry.Register(scanner.NewInjectionScanner())
	// One scanner for all entities, so that its time budget and its cap on
	// findings hold for the simulation as a whole.
	if len(pol.CustomEntities) > 0 {
		specs := make([]scanner.CustomEntitySpec, 0, len(pol.CustomEntities))
		for _, ce := range pol.CustomEntities {
			specs = append(specs, scanner.CustomEntitySpec{
				Name: ce.Name, Pattern: ce.Pattern, Description: ce.Description,
				Severity: ce.Severity, Confidence: ce.Confidence,
			})
		}
		registry.Register(scanner.NewCustomScanner(func() []scanner.CustomEntitySpec { return specs }))
	}
	// Include competitor scanner in simulation so users can preview
	// competitor detection findings alongside PII/secret/injection.
	if len(pol.Competitors) > 0 {
		compSpecs := make([]scanner.CompetitorSpec, 0, len(pol.Competitors))
		for _, c := range pol.Competitors {
			compSpecs = append(compSpecs, scanner.CompetitorSpec{
				Name: c.Name, Patterns: c.Patterns,
				Severity: c.Severity, Action: c.Action, Enabled: c.Enabled,
			})
		}
		registry.Register(scanner.NewCompetitorScanner(func() []scanner.CompetitorSpec { return compSpecs }))
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	findings, err := registry.ScanAll(ctx, []byte(body.SampleText))
	if err != nil {
		// A scan that was cut short (too slow, too many matches) has no
		// verdict to show; answering with what it found so far would read
		// as one.
		if errors.Is(err, scanner.ErrScanDegraded) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "the simulation could not be completed: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Map each finding to the resulting action per policy.
	out := make([]map[string]interface{}, 0, len(findings))
	topAction := policy.ActionPass
	for _, f := range findings {
		act := policy.ActionPass
		if rule, ok := pol.MatchedRule(f); ok {
			act = rule.Action
		}
		if policyActionSeverity(act) > policyActionSeverity(topAction) {
			topAction = act
		}
		out = append(out, map[string]interface{}{
			"type":       f.Type,
			"category":   f.Category,
			"severity":   f.Severity,
			"match":      simulateMatch(f),
			"confidence": f.Confidence,
			"action":     string(act),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"policy_name":    pol.Name,
		"policy_version": pol.Version,
		"action":         string(topAction),
		"findings":       out,
	})
}

// simulateMatch is the match as the simulation shows it. What a pattern of
// the policy matched is masked by its scanner; this makes sure of it for
// the two types whose patterns the caller wrote.
func simulateMatch(f scanner.Finding) string {
	if f.Type == "custom" || f.Type == "competitor" {
		return scanner.MaskMatch(f.Match)
	}
	return f.Match
}

func policyActionSeverity(a policy.Action) int {
	switch a {
	case policy.ActionBlock:
		return 5
	case policy.ActionStrip:
		return 4
	case policy.ActionRedact:
		return 3
	case policy.ActionWarn:
		return 2
	case policy.ActionLog:
		return 1
	}
	return 0
}

// atomicWriteFile replaces the policy file; see writePolicyFile.
func atomicWriteFile(path string, data []byte) error {
	return writePolicyFile(path, data)
}

// handleCustomEntityList returns the custom_entities array from the active policy.
func (cfg Config) handleCustomEntityList(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy store unavailable"})
		return
	}
	p := cfg.PolicyStore.GetPolicy()
	if p == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no policy loaded"})
		return
	}
	entities := p.CustomEntities
	if entities == nil {
		entities = []policy.CustomEntity{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": entities, "total": len(entities)})
}

// handleCustomEntityCreate adds a custom entity to the policy YAML and reloads.
func (cfg Config) handleCustomEntityCreate(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy store unavailable"})
		return
	}
	// Tier enforcement: higher tiers may disable custom entities.
	if cfg.TierEnforcer != nil && !cfg.TierEnforcer.CustomEntitiesAllowed() {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "custom entities not available on " + cfg.TierEnforcer.TierName() + " tier",
		})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var entity policy.CustomEntity
	r.Body = http.MaxBytesReader(w, r.Body, maxEntityBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&entity); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body exceeds 16384 bytes"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Pattern = strings.TrimSpace(entity.Pattern)
	if entity.Name == "" || entity.Pattern == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and pattern are required"})
		return
	}
	if err := scanner.CheckPattern(entity.Pattern); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid regex: " + err.Error()})
		return
	}
	pol, err := policy.LoadFromFile(cfg.PolicyPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, ce := range pol.CustomEntities {
		if ce.Name == entity.Name {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "entity with this name already exists"})
			return
		}
	}
	pol.CustomEntities = append(pol.CustomEntities, entity)
	updated, err := yaml.Marshal(pol)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// The policy with the entity in it must be one that loads: the entity's
	// name, and how many entities there are, are judged there. Writing a
	// file the proxy then refuses would leave it unable to restart.
	if _, err := policy.LoadFromBytes(updated); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := atomicWriteFile(cfg.PolicyPath, updated); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload: " + err.Error()})
		return
	}
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.CompetitorScanner != nil {
		cfg.CompetitorScanner.Refresh()
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true, "entity": entity})
}

// handleCustomEntityDelete removes a custom entity by name and reloads.
func (cfg Config) handleCustomEntityDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if strings.TrimSpace(name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if cfg.PolicyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy store unavailable"})
		return
	}
	pol, err := policy.LoadFromFile(cfg.PolicyPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	found := false
	filtered := make([]policy.CustomEntity, 0, len(pol.CustomEntities))
	for _, ce := range pol.CustomEntities {
		if ce.Name == name {
			found = true
		} else {
			filtered = append(filtered, ce)
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "entity not found"})
		return
	}
	pol.CustomEntities = filtered
	updated, err := yaml.Marshal(pol)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := atomicWriteFile(cfg.PolicyPath, updated); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload: " + err.Error()})
		return
	}
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.CompetitorScanner != nil {
		cfg.CompetitorScanner.Refresh()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
