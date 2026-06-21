package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	// Persist atomically: write to tmp then rename.
	dir := filepath.Dir(cfg.PolicyPath)
	tmp, err := os.CreateTemp(dir, ".tamga-policy-*.yaml")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tmp.WriteString(yamlText); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = tmp.Close()
	if err := os.Rename(tmp.Name(), cfg.PolicyPath); err != nil {
		_ = os.Remove(tmp.Name())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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

// handlePolicySimulate runs the full scan + policy pipeline on user-supplied
// YAML and sample text without writing anything to disk.
//
// Body: { "yaml": "...", "sample_text": "..." }
func (cfg Config) handlePolicySimulate(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var body struct {
		YAML       string `json:"yaml"`
		SampleText string `json:"sample_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	for _, ce := range pol.CustomEntities {
		registry.Register(scanner.NewCustomScanner(func() []scanner.CustomEntitySpec {
			return []scanner.CustomEntitySpec{{
				Name: ce.Name, Pattern: ce.Pattern, Description: ce.Description,
				Severity: ce.Severity, Confidence: ce.Confidence,
			}}
		}))
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
			"match":      f.Match,
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

func policyActionSeverity(a policy.Action) int {
	switch a {
	case policy.ActionBlock:
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

// atomicWriteFile writes data to path via a temp file + rename (crash-safe).
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tamga-policy-*.yaml")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	_ = tmp.Close()
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
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
	if err := json.NewDecoder(r.Body).Decode(&entity); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Pattern = strings.TrimSpace(entity.Pattern)
	if entity.Name == "" || entity.Pattern == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and pattern are required"})
		return
	}
	if _, err := regexp.Compile(entity.Pattern); err != nil {
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
	if err := atomicWriteFile(cfg.PolicyPath, updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
