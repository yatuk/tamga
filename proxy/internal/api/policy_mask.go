package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/policy/history"
)

// A rule's notify webhook is either a reference to an environment variable,
// "${SLACK_WEBHOOK_URL}", or the URL itself. The URL of a Slack, Teams or
// Discord webhook is its credential: whoever has it can post there. Every
// role reads the policy, so the URL is shown to an admin only, the role that
// may change it (see policy.PrivilegedChanges). Everyone else gets scheme
// and host, and notifyMask where the rest was. A reference names a variable
// and holds no secret; it is shown as it is.
//
// Nothing else in a policy is a credential: provider endpoints are refused
// with userinfo or a query (policy.ErrEndpointURL), api_key_env is the name
// of a variable, and exceptions, custom entities and competitors hold
// detection content.
const (
	notifyMask = "***masked***"

	// maskedHeader is set on a response in which something was masked, to
	// what was.
	maskedHeader = "X-Tamga-Masked"
	maskedNotify = "notify.webhook"

	codeMaskedValue = "masked_value"
)

// notifyReference reports whether a webhook value is a "${NAME}" reference,
// by the test policy.notifyURL applies before it looks anything up.
func notifyReference(v string) bool {
	v = strings.TrimSpace(v)
	return strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}")
}

// maskNotifyURL returns what a caller below admin sees in place of a
// webhook value. Empty values, references and values already masked are
// returned as they are.
func maskNotifyURL(v string) string {
	t := strings.TrimSpace(v)
	if t == "" || notifyReference(t) || strings.Contains(t, notifyMask) {
		return v
	}
	u, err := url.Parse(t)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return notifyMask
	}
	// Host without userinfo; the port is kept.
	return u.Scheme + "://" + u.Host + "/" + notifyMask
}

// seesNotifyURLs reports whether the caller is shown webhook URLs.
func seesNotifyURLs(r *http.Request) bool {
	return strings.EqualFold(RoleFrom(r.Context()), "admin")
}

// markMasked notes on the response that it holds masked values.
func markMasked(w http.ResponseWriter) {
	w.Header().Set(maskedHeader, maskedNotify)
}

// maskedPolicy returns p for the caller: p itself for an admin, and for
// anyone else a copy whose notify webhooks are masked. The active policy is
// shared with the proxy path and is never written to.
func maskedPolicy(w http.ResponseWriter, r *http.Request, p *policy.Policy) *policy.Policy {
	if p == nil || seesNotifyURLs(r) {
		return p
	}
	out := *p
	out.Rules = make(map[string]policy.Rule, len(p.Rules))
	masked := false
	for name, rule := range p.Rules {
		if len(rule.Notify) > 0 {
			notify := make([]policy.Notify, len(rule.Notify))
			for i, n := range rule.Notify {
				if m := maskNotifyURL(n.Webhook); m != n.Webhook {
					n.Webhook = m
					masked = true
				}
				notify[i] = n
			}
			rule.Notify = notify
		}
		out.Rules[name] = rule
	}
	if masked {
		markMasked(w)
	}
	return &out
}

// maskedYAML returns policy text for the caller: the text itself for an
// admin, and for anyone else the text with its webhook URLs masked.
func maskedYAML(w http.ResponseWriter, r *http.Request, text string) string {
	if seesNotifyURLs(r) {
		return text
	}
	out := maskPolicyYAML(text)
	if out != text {
		markMasked(w)
	}
	return out
}

func maskedRevision(w http.ResponseWriter, r *http.Request, rev history.Revision) history.Revision {
	rev.YAML = maskedYAML(w, r, rev.YAML)
	return rev
}

func maskedProposal(w http.ResponseWriter, r *http.Request, p history.Proposal) history.Proposal {
	p.YAML = maskedYAML(w, r, p.YAML)
	return p
}

// anyURL finds URLs in text that could not be read as YAML.
var anyURL = regexp.MustCompile(`https?://[^\s"'<>]+`)

// maskPolicyYAML masks the webhook URLs in policy text and leaves the rest
// of the text, comments and layout included, as it was written.
//
// The values come from the parsed text (every "webhook" key, an alias
// followed to its anchor) and each is then replaced wherever the text holds
// it, so a comment that repeats the URL is covered too. A value that is not
// in the text as it was parsed (an escape in a quoted scalar, a scalar
// folded over lines) cannot be replaced that way: the documents are then
// written out again from their nodes, with the values masked and the layout
// lost. Text that is not YAML at all, which only an old revision can be,
// has every URL in it masked.
func maskPolicyYAML(text string) string {
	docs, err := yamlDocuments(text)
	if err != nil {
		return anyURL.ReplaceAllStringFunc(text, maskNotifyURL)
	}
	secrets := map[string]bool{}
	for _, doc := range docs {
		webhookScalars(doc, func(n *yaml.Node) {
			if maskNotifyURL(n.Value) != n.Value {
				secrets[strings.TrimSpace(n.Value)] = true
			}
		})
	}
	if len(secrets) == 0 {
		return text
	}
	// Longest first: one URL may be the beginning of another.
	values := make([]string, 0, len(secrets))
	for v := range secrets {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	out := text
	for _, v := range values {
		if replaceable(v) {
			out = strings.ReplaceAll(out, v, maskNotifyURL(v))
		}
	}
	if !holdsWebhookURL(out) {
		return out
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	for _, doc := range docs {
		webhookScalars(doc, func(n *yaml.Node) {
			if m := maskNotifyURL(n.Value); m != n.Value {
				n.Value = m
				// The value no longer needs the style that hid it.
				n.Style = 0
			}
		})
		if err := enc.Encode(doc); err != nil {
			return "# The policy text is withheld: it holds a webhook URL that could not be masked.\n"
		}
	}
	_ = enc.Close()
	// What was masked is gone from the values; a comment may still spell it.
	rewritten := buf.String()
	for _, v := range values {
		if replaceable(v) {
			rewritten = strings.ReplaceAll(rewritten, v, maskNotifyURL(v))
		}
	}
	return rewritten
}

// replaceable reports whether a webhook value is distinctive enough to be
// replaced wherever the text holds it. A value that is no URL (a word, a
// typo) could be part of anything else in the text.
func replaceable(v string) bool {
	return len(v) >= 12 && strings.Contains(v, "://")
}

// holdsWebhookURL reports whether text still holds a webhook URL in the
// clear, or cannot be read to find out.
func holdsWebhookURL(text string) bool {
	docs, err := yamlDocuments(text)
	if err != nil {
		return true
	}
	found := false
	for _, doc := range docs {
		webhookScalars(doc, func(n *yaml.Node) {
			if maskNotifyURL(n.Value) != n.Value {
				found = true
			}
		})
	}
	return found
}

// yamlDocuments parses every YAML document in text. A policy is one
// document, but text from before that was enforced may hold more.
func yamlDocuments(text string) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}
}

// webhookScalars calls visit for the scalar value of every "webhook" key
// below n, wherever it is: text that does not have the shape of a policy is
// masked all the same.
func webhookScalars(n *yaml.Node, visit func(*yaml.Node)) {
	walkWebhooks(n, visit, map[*yaml.Node]bool{})
}

func walkWebhooks(n *yaml.Node, visit func(*yaml.Node), seen map[*yaml.Node]bool) {
	if n == nil || seen[n] {
		return
	}
	seen[n] = true
	if n.Kind == yaml.AliasNode {
		walkWebhooks(n.Alias, visit, seen)
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			if key.Kind == yaml.ScalarNode && strings.EqualFold(strings.TrimSpace(key.Value), "webhook") {
				for val != nil && val.Kind == yaml.AliasNode {
					val = val.Alias
				}
				if val != nil && val.Kind == yaml.ScalarNode {
					visit(val)
					continue
				}
			}
			walkWebhooks(n.Content[i+1], visit, seen)
		}
		return
	}
	for _, c := range n.Content {
		walkWebhooks(c, visit, seen)
	}
}

// refuseMaskedNotify answers 422 and returns true when a policy someone
// wants to install holds the mask where a webhook URL belongs: text that
// was read by a caller below admin and is being written back. Installing it
// would replace the URL with the mask and silence the alert. The privilege
// split refuses the same text from anyone but an admin (notify would
// change); this refuses it from an admin too, and says why.
func refuseMaskedNotify(w http.ResponseWriter, pol *policy.Policy) bool {
	var fields []string
	for name, rule := range pol.Rules {
		for _, n := range rule.Notify {
			if strings.Contains(n.Webhook, notifyMask) {
				fields = append(fields, "rules."+name+".notify")
				break
			}
		}
	}
	if len(fields) == 0 {
		return false
	}
	sort.Strings(fields)
	writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
		"error": "the policy holds a masked webhook URL (" + notifyMask + ") in " + strings.Join(fields, ", ") +
			": this text was read without the admin role, which hides webhook URLs. " +
			"Write the URL itself or, better, a reference such as ${SLACK_WEBHOOK_URL}.",
		"code":   codeMaskedValue,
		"fields": fields,
	})
	return true
}
