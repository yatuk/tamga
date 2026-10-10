package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/policy"
)

// The secret part of the webhook URL in govBasePolicy, and what a caller
// below admin sees in its place.
const (
	govHookSecret = "hook-SECRET"
	govHookMasked = "https://example.invalid/" + notifyMask
)

// viewerKey adds a read-scope key, which acts as a viewer.
func (f *govFixture) viewerKey() string {
	f.t.Helper()
	created, err := f.cfg.APIKeys.(*apikeys.MemoryStore).Create("viewer", apikeys.ScopeRead)
	if err != nil {
		f.t.Fatal(err)
	}
	return created.RawKey
}

// policyReads are the four routes that return policy text, after the
// fixture has a revision and a proposal that hold the webhook URL.
func policyReads(t *testing.T, f *govFixture) []string {
	t.Helper()
	raw, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(raw), "sensitivity: medium", "sensitivity: high", 1)
	put := f.put(f.adminA.RawKey, changed)
	revID, _ := put.body["revision_id"].(string)
	if put.status != http.StatusOK || revID == "" {
		t.Fatalf("seed revision: %d %s", put.status, put.raw)
	}
	body, _ := json.Marshal(map[string]string{"message": "m", "yaml": strings.Replace(changed, "action: BLOCK", "action: WARN", 1)})
	if res := f.do(http.MethodPost, "/policies/proposals", f.adminA.RawKey, string(body), nil); res.status != http.StatusCreated {
		t.Fatalf("seed proposal: %d %s", res.status, res.raw)
	}
	return []string{"/policies", "/policies/history", "/policies/revisions/" + revID, "/policies/proposals"}
}

func TestNotifyURLIsMaskedBelowAdmin(t *testing.T) {
	f := newGovFixture(t, govBasePolicy)
	paths := policyReads(t, f)
	for _, path := range paths {
		for who, key := range map[string]string{"viewer": f.viewerKey(), "analyst": f.analystA.RawKey} {
			req := f.request(http.MethodGet, path, key)
			if req.status != http.StatusOK {
				t.Fatalf("%s GET %s: %d %s", who, path, req.status, req.raw)
			}
			if strings.Contains(req.raw, govHookSecret) {
				t.Errorf("%s GET %s shows the webhook URL: %s", who, path, req.raw)
			}
			if !strings.Contains(req.raw, govHookMasked) {
				t.Errorf("%s GET %s: no masked URL (scheme and host, then the mask) in %s", who, path, req.raw)
			}
			if got := req.header.Get(maskedHeader); got != maskedNotify {
				t.Errorf("%s GET %s: %s = %q, want %q", who, path, maskedHeader, got, maskedNotify)
			}
		}
		for who, key := range map[string]string{"admin key": f.adminA.RawKey, "static admin key": govAdminKey} {
			req := f.request(http.MethodGet, path, key)
			if !strings.Contains(req.raw, govHookSecret) || strings.Contains(req.raw, notifyMask) {
				t.Errorf("%s GET %s does not show the policy as it is: %s", who, path, req.raw)
			}
			if got := req.header.Get(maskedHeader); got != "" {
				t.Errorf("%s GET %s: %s = %q on an unmasked answer", who, path, maskedHeader, got)
			}
		}
	}
	// Reading masked the answer, not the policy in force or the file.
	if hook := f.cfg.PolicyStore.GetPolicy().Rules["pii_detection"].Notify[0].Webhook; !strings.Contains(hook, govHookSecret) {
		t.Fatalf("the active policy's webhook is now %q", hook)
	}
	if raw, _ := os.ReadFile(f.path); !strings.Contains(string(raw), govHookSecret) {
		t.Fatal("the policy file lost its webhook URL")
	}
}

// govHeaderResponse is a govResponse with the response headers.
type govHeaderResponse struct {
	govResponse
	header http.Header
}

// request is do for a read, keeping the headers.
func (f *govFixture) request(method, path, key string) govHeaderResponse {
	f.t.Helper()
	req, _ := http.NewRequest(method, "/api/v1"+path, nil)
	req.Header.Set("X-Tamga-Admin-Key", key)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	out := govHeaderResponse{header: rec.Header()}
	out.status, out.raw = rec.Code, rec.Body.String()
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	return out
}

// A reference names an environment variable and is no secret: everyone
// reads it as it is written, and nothing is marked as masked.
func TestNotifyReferenceIsNotMasked(t *testing.T) {
	const ref = "${SLACK_WEBHOOK_URL}"
	f := newGovFixture(t, strings.Replace(govBasePolicy, "https://example.invalid/hook-SECRET", `"`+ref+`"`, 1))
	for _, path := range policyReads(t, f) {
		for who, key := range map[string]string{"viewer": f.viewerKey(), "analyst": f.analystA.RawKey, "admin": f.adminA.RawKey} {
			res := f.request(http.MethodGet, path, key)
			if !strings.Contains(res.raw, ref) || strings.Contains(res.raw, notifyMask) {
				t.Errorf("%s GET %s: the reference was changed: %s", who, path, res.raw)
			}
			if got := res.header.Get(maskedHeader); got != "" {
				t.Errorf("%s GET %s: %s = %q with nothing masked", who, path, maskedHeader, got)
			}
		}
	}
}

// What a caller below admin read cannot be written back over the real URL:
// not by an analyst, whose change of notify the privilege split refuses
// anyway, and not by an admin who was handed the text.
func TestMaskedPolicyCannotBeWrittenBack(t *testing.T) {
	f := newGovFixture(t, govBasePolicy)
	paths := policyReads(t, f)
	rev := f.request(http.MethodGet, paths[2], f.analystA.RawKey)
	masked, _ := rev.body["yaml"].(string)
	if !strings.Contains(masked, govHookMasked) || strings.Contains(masked, govHookSecret) {
		t.Fatalf("the analyst's copy of the revision: %s", masked)
	}
	// The analyst edits what an analyst may edit, in the copy it was given.
	edited := strings.Replace(masked, "action: BLOCK", "action: WARN", 1)
	if edited == masked {
		t.Fatal("the edit changed nothing")
	}
	before, _ := os.ReadFile(f.path)
	proposal, _ := json.Marshal(map[string]string{"message": "m", "yaml": edited})

	for who, key := range map[string]string{"analyst": f.analystA.RawKey, "admin": f.adminA.RawKey, "static admin key": govAdminKey} {
		for name, res := range map[string]govResponse{
			"PUT":      f.put(key, edited),
			"proposal": f.do(http.MethodPost, "/policies/proposals", key, string(proposal), nil),
		} {
			if res.status != http.StatusUnprocessableEntity || res.code() != codeMaskedValue {
				t.Errorf("%s %s of a masked policy: %d %s, want 422 %s", who, name, res.status, res.raw, codeMaskedValue)
			}
			if !strings.Contains(res.raw, "rules.pii_detection.notify") {
				t.Errorf("%s %s: the refusal does not name the rule: %s", who, name, res.raw)
			}
		}
	}
	if after, _ := os.ReadFile(f.path); string(after) != string(before) {
		t.Fatal("a refused write changed the policy file")
	}
	pol := f.cfg.PolicyStore.GetPolicy()
	if hook := pol.Rules["pii_detection"].Notify[0].Webhook; !strings.Contains(hook, govHookSecret) {
		t.Fatalf("the webhook in force is now %q", hook)
	}
	if pol.Rules["secret_detection"].Action != policy.ActionBlock {
		t.Fatal("part of the refused policy was installed")
	}
	// The mask moved to a rule that had no notify is refused the same way.
	moved := strings.Replace(govBasePolicy, "    action: BLOCK\n", "    action: BLOCK\n    notify:\n      - channel: slack\n        webhook: "+govHookMasked+"\n", 1)
	if res := f.put(f.adminA.RawKey, moved); res.status != http.StatusUnprocessableEntity || res.code() != codeMaskedValue {
		t.Errorf("mask in another rule: %d %s", res.status, res.raw)
	}
}

func TestMaskNotifyURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://hooks.slack.com/services/T000/B000/XXXXXXXX": "https://hooks.slack.com/" + notifyMask,
		"https://discord.com/api/webhooks/1/abc?wait=true":    "https://discord.com/" + notifyMask,
		"https://user:pass@hooks.example.com:8443/x":          "https://hooks.example.com:8443/" + notifyMask,
		"http://10.0.0.5/hook":                                "http://10.0.0.5/" + notifyMask,
		"https://hooks.example.com":                           "https://hooks.example.com/" + notifyMask,
		"not a url":                                           notifyMask,
		"ftp://example.com/x":                                 notifyMask,
		"${SLACK_WEBHOOK_URL}":                                "${SLACK_WEBHOOK_URL}",
		"  ${TEAMS_WEBHOOK_URL}  ":                            "  ${TEAMS_WEBHOOK_URL}  ",
		"":                                                    "",
		"https://hooks.slack.com/" + notifyMask:               "https://hooks.slack.com/" + notifyMask,
	} {
		if got := maskNotifyURL(in); got != want {
			t.Errorf("maskNotifyURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// However the URL is written, the text a caller below admin gets does not
// hold it; whatever else the text holds is still there.
func TestMaskPolicyYAML(t *testing.T) {
	const secret = "T000/B000/SECRETSECRET"
	const hook = "https://hooks.slack.com/services/" + secret
	const masked = "https://hooks.slack.com/" + notifyMask
	rule := func(webhook string) string {
		return "version: \"1.0\"\nname: p\n# posts to the security channel\nrules:\n  pii_detection:\n    action: REDACT\n    notify:\n      - channel: slack\n        webhook: " + webhook + "\n  secret_detection:\n    action: BLOCK\n"
	}
	cases := map[string]string{
		"plain":                                 rule(hook),
		"double quoted":                         rule(`"` + hook + `"`),
		"single quoted":                         rule(`'` + hook + `'`),
		"with a comment":                        rule(hook + "   # was " + hook),
		"flow style":                            "name: p\nrules:\n  pii_detection: {action: REDACT, notify: [{channel: slack, webhook: \"" + hook + "\"}]}\n",
		"escaped":                               rule(`"https://hooks.slack.com/services\u002FT000/B000/SECRETSECRET"`),
		"folded":                                rule(">-\n          https://hooks.slack.com/services/T000/B000/\n          SECRETSECRET"),
		"anchor and alias":                      "name: p\nrules:\n  a:\n    action: WARN\n    notify:\n      - webhook: &hook " + hook + "\n  b:\n    action: WARN\n    notify:\n      - webhook: *hook\n",
		"two documents":                         rule("${SLACK_WEBHOOK_URL}") + "---\nrules:\n  x:\n    notify:\n      - webhook: " + hook + "\n",
		"two hooks, one the start of the other": rule(hook) + "  third:\n    action: WARN\n    notify:\n      - webhook: " + hook + "/more\n",
		"not YAML":                              "rules:\n  pii: [unclosed\n    webhook: " + hook + "\n",
		"not a policy":                          "anything:\n  - deeper:\n      webhook: " + hook + "\n",
	}
	for name, text := range cases {
		out := maskPolicyYAML(text)
		if strings.Contains(out, "SECRETSECRET") || strings.Contains(out, "T000") {
			t.Errorf("%s: the URL is still in the text:\n%s", name, out)
		}
		if !strings.Contains(out, masked) {
			t.Errorf("%s: no masked URL in:\n%s", name, out)
		}
	}

	// Where the URL is in the text as it is read, only the URL changes:
	// comments and layout stay.
	plain := rule(hook)
	if got, want := maskPolicyYAML(plain), strings.ReplaceAll(plain, hook, masked); got != want {
		t.Errorf("plain text was changed beyond its URL:\n%s", got)
	}
	// Nothing to mask: the text is returned as it is.
	for _, text := range []string{rule("${SLACK_WEBHOOK_URL}"), rule(`""`), "name: p\nrules: {}\n", ""} {
		if got := maskPolicyYAML(text); got != text {
			t.Errorf("text without a webhook URL was changed:\n%q\n%q", text, got)
		}
	}
	// Other URLs in a policy are not credentials and stay.
	withEndpoint := rule(hook) + "providers:\n  pools:\n    openai:\n      strategy: fallback_chain\n      providers:\n        - name: a\n          base_url: https://api.openai.com\n"
	if out := maskPolicyYAML(withEndpoint); !strings.Contains(out, "base_url: https://api.openai.com") {
		t.Errorf("a provider endpoint was masked:\n%s", out)
	}
	// A webhook value that is no URL is masked whole, and nothing else that
	// happens to contain the same letters is touched.
	odd := rule("slack")
	out := maskPolicyYAML(odd)
	if !strings.Contains(out, "channel: slack") || !strings.Contains(out, notifyMask) || strings.Contains(out, "webhook: slack") {
		t.Errorf("a webhook that is no URL:\n%s", out)
	}
}
