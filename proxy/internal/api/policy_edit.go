package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

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
		writePolicyLoadError(w, err)
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
	policyWriteMu.Lock()
	defer policyWriteMu.Unlock()
	// Under dual control nothing is written without a second person, and
	// this path has only the caller.
	if cfg.refuseDirectWrite(w, r) {
		return
	}
	pol := loadPolicyForWrite(w, yamlText)
	if pol == nil {
		return
	}
	if cfg.refusePrivileged(w, r, pol) {
		return
	}
	before := cfg.PolicyStore.GetPolicy()
	if err := writePolicyFile(cfg.PolicyPath, []byte(yamlText)); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload: " + err.Error()})
		return
	}
	cfg.refreshPolicyScanners()
	pol = cfg.PolicyStore.GetPolicy()
	// Into the history as well as the audit log, as an approved proposal
	// is: a revision for every policy that was in force.
	revID, changed := recordPolicyChange(cfg.Audit, cfg.PolicyHistory, policyChange{
		kind: "policy.put", source: sourcePut, actor: actorFromRequest(r),
		message: "direct write (PUT /policies)", yaml: yamlText,
		old: before, new: pol,
		detail: map[string]interface{}{"size": len(yamlText)},
	})
	out := map[string]interface{}{"ok": true, "name": pol.Name, "version": pol.Version, "changed": changed}
	if revID != "" {
		out["revision_id"] = revID
	}
	writeJSON(w, http.StatusOK, out)
}

// A simulation takes a policy of up to 256 KiB, as PUT /policies does, and a
// sample text of up to 256 KiB, as POST /api/v1/scan does. The built-in
// scanners run one after another here and need about a second and a half
// for that much text on an idle core, more on a busy one.
const (
	maxSimulateBodyBytes = 512 << 10
	maxSimulateTextBytes = 256 << 10
)

// defaultSimulateTimeout is the scan time a simulation gets. It is the
// proxy's own default for a scan (TAMGA_SCANNER_PIPELINE_TIMEOUT_MS): two
// seconds, the limit before, was less than the largest allowed sample needs
// on a machine that is doing anything else, and the endpoint then answered
// 503 for a request it had accepted.
const defaultSimulateTimeout = 5 * time.Second

func (cfg Config) simulateTimeout() time.Duration {
	if cfg.SimulateTimeout > 0 {
		return cfg.SimulateTimeout
	}
	return defaultSimulateTimeout
}

// maxEntityBodyBytes bounds the body of a custom entity create.
const maxEntityBodyBytes = 16 << 10

// handlePolicySimulate runs the full scan + policy pipeline on user-supplied
// YAML and sample text without writing anything to disk.
//
// The policy is the caller's, and so are its regular expressions: it is
// loaded like any other, so a pattern past the limits is refused, and the
// pattern scanners stop at the request's deadline. A match is returned
// masked, as it would be stored, with its place in sample_text (see
// FindingSpan).
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
	ctx, cancel := context.WithTimeout(r.Context(), cfg.simulateTimeout())
	defer cancel()
	// As one piece of text, settled the way the proxy path settles a
	// segment: a value found in several views of the text is reported once,
	// and a match made only in a normalised view carries no position.
	findings, err := registry.ScanRaw(ctx, []byte(body.SampleText), scanner.PipelineConfig{Mode: scanner.ModeAdaptive})
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
		// The action the engine gives the finding: the stronger of its two
		// rule keys and, for a custom entity or a competitor, the entry's
		// own action. No role is given, so no exception applies.
		act := pol.FindingAction(f, "", true)
		if policyActionSeverity(act) > policyActionSeverity(topAction) {
			topAction = act
		}
		item := map[string]interface{}{
			"type":       f.Type,
			"category":   f.Category,
			"severity":   f.Severity,
			"match":      simulateMatch(f),
			"confidence": f.Confidence,
			"action":     string(act),
		}
		// Where it is in sample_text, which the caller has: the match is
		// masked, so it cannot be found again by looking for it.
		FindingSpan(body.SampleText, f).Put(item)
		out = append(out, item)
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
	// The console sends the action in lower case. It is written to the file
	// under its name; a value that is no action is refused when the policy
	// with the entity in it is loaded, below.
	entity.Action = strings.ToUpper(strings.TrimSpace(entity.Action))
	if entity.Name == "" || entity.Pattern == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and pattern are required"})
		return
	}
	if err := scanner.CheckPattern(entity.Pattern); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid regex: " + err.Error()})
		return
	}
	policyWriteMu.Lock()
	defer policyWriteMu.Unlock()
	// Adding an entity rewrites the policy, so it is a direct write.
	if cfg.refuseDirectWrite(w, r) {
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
	next, err := policy.LoadFromBytes(updated)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// The file is rewritten whole. Should it differ from the active policy
	// in more than the entity, the rest is judged like any other write.
	if cfg.refusePrivileged(w, r, next) {
		return
	}
	before := cfg.PolicyStore.GetPolicy()
	if err := atomicWriteFile(cfg.PolicyPath, updated); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload: " + err.Error()})
		return
	}
	cfg.refreshPolicyScanners()
	recordPolicyChange(cfg.Audit, cfg.PolicyHistory, policyChange{
		kind: "policy.custom_entity.add", source: sourceEntityAdd, actor: actorFromRequest(r),
		target: entity.Name, message: "add custom entity " + entity.Name, yaml: string(updated),
		old: before, new: cfg.PolicyStore.GetPolicy(),
	})
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
	policyWriteMu.Lock()
	defer policyWriteMu.Unlock()
	if cfg.refuseDirectWrite(w, r) {
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
	if cfg.refusePrivileged(w, r, pol) {
		return
	}
	before := cfg.PolicyStore.GetPolicy()
	if err := atomicWriteFile(cfg.PolicyPath, updated); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload: " + err.Error()})
		return
	}
	cfg.refreshPolicyScanners()
	recordPolicyChange(cfg.Audit, cfg.PolicyHistory, policyChange{
		kind: "policy.custom_entity.delete", source: sourceEntityDelete, actor: actorFromRequest(r),
		target: name, message: "delete custom entity " + name, yaml: string(updated),
		old: before, new: cfg.PolicyStore.GetPolicy(),
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
