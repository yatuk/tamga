package scanner

import (
	"bytes"
	"context"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yatuk/tamga/internal/scanner/normalize"

	"github.com/yatuk/tamga/internal/scanner/tckn"
)

// piiPattern describes a compiled PII detector with optional post-validation.
type piiPattern struct {
	Name       string
	Regex      *regexp.Regexp
	Severity   string
	confidence float64
	validate   func(string) bool
}

var (
	piiPatterns []piiPattern
	ipv4Regex   *regexp.Regexp
	piiOnce     sync.Once
)

func compilePIIPatterns() {
	// Octets allow single-digit values (e.g. 8.8.8.8); invalid ranges are dropped by net.ParseIP.
	ipv4Regex = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`)

	piiPatterns = []piiPattern{
		{
			Name:     "credit_card",
			Regex:    regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b|\b\d{13,19}\b`),
			Severity: "critical",
			validate: func(m string) bool {
				d := digitsOnly(m)
				if len(d) < 13 || len(d) > 19 {
					return false
				}
				return validLuhn(d)
			},
			confidence: 0.95,
		},
		{
			Name:       "ssn",
			Regex:      regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			Severity:   "critical",
			validate:   nil,
			confidence: 0.88,
		},
		{
			Name:     "tc_kimlik",
			Regex:    regexp.MustCompile(`\b[1-9]\d{10}\b`),
			Severity: "critical",
			validate: func(m string) bool {
				d := digitsOnly(m)
				if len(d) != 11 {
					return false
				}
				return validTCKN(d)
			},
			confidence: 0.95,
		},
		{
			Name: "iban",
			Regex: regexp.MustCompile(
				`(?i)\b[A-Z]{2}\d{2}(?:\s?[A-Z0-9]){11,30}\b`,
			),
			Severity: "critical",
			validate: func(m string) bool {
				return validIBAN(m)
			},
			confidence: 0.95,
		},
		{
			Name: "email",
			Regex: regexp.MustCompile(
				`(?i)\b[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\b`,
			),
			Severity: "high",
			validate: func(m string) bool {
				return validEmailRFC5322Pragmatic(m)
			},
			confidence: 0.82,
		},
		{
			Name: "phone_tr",
			Regex: regexp.MustCompile(
				// No leading \b: '+' is non-word; "Ara +90 ..." would otherwise miss.
				`(?i)(?:\+90|0)(?:\s|-)?5\d{2}(?:\s|-)?\d{3}(?:\s|-)?\d{2}(?:\s|-)?\d{2}\b`,
			),
			Severity: "high",
			validate: func(m string) bool {
				return validTRMobile(m)
			},
			confidence: 0.88,
		},
		{
			Name:       "passport_number",
			Regex:      regexp.MustCompile(`(?i)\b(passport\s*(no|number|#)?[:\s]*[A-Z0-9]{6,12})\b`),
			Severity:   "high",
			validate:   nil,
			confidence: 0.85,
		},
		{
			Name:     "date_of_birth",
			Regex:    regexp.MustCompile(`(?i)\b((?:DOB|birth|born|doğum\s*tarihi)[:\s]*\d{1,2}[/\-\.]\d{1,2}[/\-\.]\d{2,4})\b`),
			Severity: "high",
			validate: func(m string) bool {
				return len(m) >= 8 && len(m) <= 30
			},
			confidence: 0.75,
		},
		{
			Name:       "medical_record",
			Regex:      regexp.MustCompile(`(?i)\b(medical\s*record\s*(?:number|no|#)?[:\s]*\d{4,12})\b`),
			Severity:   "high",
			validate:   nil,
			confidence: 0.80,
		},
		{
			Name:       "npi_number",
			Regex:      regexp.MustCompile(`(?i)\b(NPI[:\s]*\d{10}|National\s+Provider\s+ID[:\s]*\d{10})\b`),
			Severity:   "high",
			validate:   nil,
			confidence: 0.85,
		},
		{
			Name:       "dea_number",
			Regex:      regexp.MustCompile(`(?i)\b(DEA\s*(?:number|#)?[:\s]*[A-Z]{2}\d{7})\b`),
			Severity:   "high",
			validate:   nil,
			confidence: 0.85,
		},
	}
}

// PIIScanner detects personally identifiable information using compiled regexp patterns.
// Patterns are compiled once (sync.Once) and reused across all requests.
type PIIScanner struct{}

// NewPIIScanner creates a scanner that detects personally identifiable information.
func NewPIIScanner() *PIIScanner {
	piiOnce.Do(compilePIIPatterns)
	return &PIIScanner{}
}

func (s *PIIScanner) Name() string { return "pii" }

func (s *PIIScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	piiOnce.Do(compilePIIPatterns)
	var findings []Finding
	contentStr := string(content)

	// Normalize to detect Unicode-evaded PII (mathematical bold, fullwidth, homoglyphs).
	normResult := normalize.Apply(contentStr, normalize.Default())
	normText := normResult.Text()
	normBytes := []byte(normText)

	for _, p := range piiPatterns {
		matches := p.Regex.FindAllIndex(content, -1)
		for _, loc := range matches {
			matched := string(content[loc[0]:loc[1]])
			if p.validate != nil && !p.validate(matched) {
				continue
			}
			var masked string
			if p.Name == "credit_card" {
				masked = maskPAN(matched)
			} else {
				masked = maskContent(matched)
			}
			score := calculatePIIConfidence(p.Name, contentStr, loc[0], matched)
			metadata := map[string]string{}
			if p.Name == "credit_card" {
				if bin := LookupBIN(matched); bin != nil {
					metadata["card_brand"] = bin.Brand
					metadata["card_type"] = bin.Type
					metadata["bank_name"] = bin.BankName
					metadata["country"] = bin.CountryCode
					metadata["bin"] = bin.BIN
				}
			}
			if p.Name == "tc_kimlik" {
				d := digitsOnly(matched)
				if reason, ok := tckn.IsDenylisted(d); ok {
					metadata["denylist_reason"] = reason
					metadata["denylist_match"] = "true"
					// Recalculate the full score so Action, Total, and Reasoning stay consistent.
					score = CalculateConfidence(ConfidenceFactor{
						Format:    WFormat,
						Algorithm: WAlgorithm,
						Context:   WContext,
						Database:  WDatabase,
					})
				}
			}
			findings = append(findings, Finding{
				Type:            "pii",
				Category:        p.Name,
				Severity:        p.Severity,
				Match:           masked,
				StartPos:        loc[0],
				EndPos:          loc[1],
				Confidence:      float64(score.Total) / 100.0,
				ConfidenceScore: &score,
				Metadata:        metadata,
			})
		}
	}

	findings = append(findings, scanIPv4(content)...)
	// Re-scan the normalized text to catch Unicode-evaded PII
	// (mathematical bold digits, fullwidth/homoglyph characters, zero-width splits).
	for _, p := range piiPatterns {
		for _, loc := range p.Regex.FindAllIndex(normBytes, -1) {
			matched := string(normBytes[loc[0]:loc[1]])
			if p.validate != nil && !p.validate(matched) {
				continue
			}
			var masked string
			if p.Name == "credit_card" {
				masked = maskPAN(matched)
			} else {
				masked = maskContent(matched)
			}
			score := calculatePIIConfidence(p.Name, normText, loc[0], matched)
			metadata := map[string]string{"detected_via": "unicode_normalization"}
			if p.Name == "credit_card" {
				if bin := LookupBIN(matched); bin != nil {
					metadata["card_brand"] = bin.Brand
					metadata["card_type"] = bin.Type
					metadata["bank_name"] = bin.BankName
					metadata["country"] = bin.CountryCode
					metadata["bin"] = bin.BIN
				}
			}
			if p.Name == "tc_kimlik" {
				d := digitsOnly(matched)
				if reason, ok := tckn.IsDenylisted(d); ok {
					metadata["denylist_reason"] = reason
					metadata["denylist_match"] = "true"
					score = CalculateConfidence(ConfidenceFactor{
						Format:    WFormat,
						Algorithm: WAlgorithm,
						Database:  WDatabase,
						Context:   WContext,
					})
				}
			}
			findings = append(findings, Finding{
				Type:            "pii",
				Category:        p.Name,
				Severity:        p.Severity,
				Match:           masked,
				StartPos:        loc[0],
				EndPos:          loc[1],
				ActionTaken:     string(ActionRedact),
				Confidence:      float64(score.Total) / 100.0,
				ConfidenceScore: &score,
				Metadata:        metadata,
			})
		}
	}

	// Scan digit-normalized variants to catch format-evaded TCKN
	// (separator-stripped and reversed digit sequences).
	digitVariants := normalize.NormalizeDigitGroups(normText)
	for vi, variant := range digitVariants {
		if vi == 0 || variant == normText {
			continue // already scanned in the normalized pass above
		}
		tag := "digit_separators_removed"
		if vi == 2 {
			tag = "reversed_digits"
		}
		variantBytes := []byte(variant)
		for _, p := range piiPatterns {
			for _, loc := range p.Regex.FindAllIndex(variantBytes, -1) {
				matched := string(variantBytes[loc[0]:loc[1]])
				if p.validate != nil && !p.validate(matched) {
					continue
				}
				masked := maskContent(matched)
				if p.Name == "credit_card" {
					masked = maskPAN(matched)
				}
				score := calculatePIIConfidence(p.Name, variant, loc[0], matched)
				metadata := map[string]string{"detected_via": tag}
				if p.Name == "credit_card" {
					if bin := LookupBIN(matched); bin != nil {
						metadata["card_brand"] = bin.Brand
						metadata["card_type"] = bin.Type
						metadata["bank_name"] = bin.BankName
						metadata["country"] = bin.CountryCode
						metadata["bin"] = bin.BIN
					}
				}
				if p.Name == "tc_kimlik" {
					d := digitsOnly(matched)
					if reason, ok := tckn.IsDenylisted(d); ok {
						metadata["denylist_reason"] = reason
						metadata["denylist_match"] = "true"
						score = CalculateConfidence(ConfidenceFactor{
							Format:    WFormat,
							Algorithm: WAlgorithm,
							Database:  WDatabase,
							Context:   WContext,
						})
					}
				}
				findings = append(findings, Finding{
					Type:            "pii",
					Category:        p.Name,
					Severity:        p.Severity,
					Match:           masked,
					StartPos:        loc[0],
					EndPos:          loc[1],
					ActionTaken:     string(ActionRedact),
					Confidence:      float64(score.Total) / 100.0,
					ConfidenceScore: &score,
					Metadata:        metadata,
				})
			}
		}
	}

	return findings, nil
}

func scanIPv4(content []byte) []Finding {
	var findings []Finding
	if ipv4Regex == nil {
		return nil
	}
	matches := ipv4Regex.FindAllIndex(content, -1)
	for _, loc := range matches {
		matched := string(content[loc[0]:loc[1]])
		ip := net.ParseIP(matched)
		if ip == nil || ip.To4() == nil {
			continue
		}
		if isPrivateOrSpecialIPv4(ip) {
			score := CalculateConfidence(ConfidenceFactor{Format: WFormat})
			findings = append(findings, Finding{
				Type:            "pii",
				Category:        "ip_private",
				Severity:        "low",
				Match:           maskContent(matched),
				StartPos:        loc[0],
				EndPos:          loc[1],
				Confidence:      float64(score.Total) / 100.0,
				ConfidenceScore: &score,
			})
		} else {
			score := CalculateConfidence(ConfidenceFactor{Format: WFormat, Context: WContext})
			findings = append(findings, Finding{
				Type:            "pii",
				Category:        "ip_public",
				Severity:        "medium",
				Match:           maskContent(matched),
				StartPos:        loc[0],
				EndPos:          loc[1],
				Confidence:      float64(score.Total) / 100.0,
				ConfidenceScore: &score,
			})
		}
	}
	return findings
}

func calculatePIIConfidence(category, content string, pos int, matched string) ConfidenceScore {
	factor := ConfidenceFactor{Format: WFormat}
	category = strings.ToLower(strings.TrimSpace(category))

	switch category {
	case "credit_card":
		d := digitsOnly(matched)
		if validLuhn(d) {
			factor.Algorithm = WAlgorithm
		}
		if LookupBIN(d) != nil {
			factor.Database = WDatabase
		}
		if hasContextNearby(content, pos, []string{"credit card", "kredi kartı", "card", "cvv", "cvc", "expiry"}) {
			factor.Context = WContext
		}
	case "tc_kimlik":
		if validTCKN(digitsOnly(matched)) {
			factor.Algorithm = WAlgorithm
		}
		if hasContextNearby(content, pos, []string{"kimlik", "tc", "tckn"}) {
			factor.Context = WContext
		}
	case "iban":
		if validIBAN(matched) {
			factor.Algorithm = WAlgorithm
		}
		if hasContextNearby(content, pos, []string{"iban", "hesap", "account"}) {
			factor.Context = WContext
		}
	case "email":
		if validEmailRFC5322Pragmatic(matched) {
			factor.Algorithm = 20
		}
		if hasContextNearby(content, pos, []string{"email", "e-posta", "mail"}) {
			factor.Context = WContext
		}
	case "phone_tr":
		if validTRMobile(matched) {
			factor.Algorithm = 20
		}
		if hasContextNearby(content, pos, []string{"telefon", "phone", "gsm", "cep"}) {
			factor.Context = WContext
		}
	default:
		// ssn/ip and other categories keep baseline format confidence for now.
	}

	return CalculateConfidence(factor)
}

func hasContextNearby(content string, pos int, keywords []string) bool {
	if hasContextNearbyDFA(content, pos, keywords) {
		return true
	}
	start := max(0, pos-60)
	end := min(len(content), pos+60)
	window := strings.ToLower(content[start:end])
	for _, kw := range keywords {
		if strings.Contains(window, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func hasContextNearbyDFA(content string, pos int, keywords []string) bool {
	dfa := LoadDFA()
	if dfa == nil || len(content) == 0 || len(keywords) == 0 {
		return false
	}
	start := max(0, pos-60)
	end := min(len(content), pos+60)
	window := []byte(content[start:end])
	matches := dfa.ScanBytes(window)
	if len(matches) == 0 {
		return false
	}
	kwSet := make(map[string]struct{}, len(keywords))
	for _, kw := range keywords {
		kwSet[strings.ToLower(strings.TrimSpace(kw))] = struct{}{}
	}
	for _, m := range matches {
		if m.Type != "context" {
			continue
		}
		if _, ok := kwSet[m.Pattern]; ok {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validLuhn(digits string) bool {
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	alt := false
	for i := len(digits) - 1; i >= 0; i-- {
		c := digits[i]
		if c < '0' || c > '9' {
			return false
		}
		n := int(c - '0')
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}

// validTCKN validates an 11-digit Turkish national ID number (algorithmic).
func validTCKN(d string) bool {
	if len(d) != 11 {
		return false
	}
	if d[0] == '0' {
		return false
	}
	digs := make([]int, 11)
	for i := 0; i < 11; i++ {
		if d[i] < '0' || d[i] > '9' {
			return false
		}
		digs[i] = int(d[i] - '0')
	}
	odd := digs[0] + digs[2] + digs[4] + digs[6] + digs[8]
	even := digs[1] + digs[3] + digs[5] + digs[7]
	v := ((odd*7-even)%10 + 10) % 10
	if v != digs[9] {
		return false
	}
	sum10 := 0
	for i := 0; i < 10; i++ {
		sum10 += digs[i]
	}
	return sum10%10 == digs[10]
}

// validIBAN validates IBAN (ISO 13616) check digits (mod 97).
func validIBAN(raw string) bool {
	s := strings.ReplaceAll(raw, " ", "")
	s = strings.ToUpper(s)
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	if len(s) < 4 {
		return false
	}
	rearranged := s[4:] + s[:4]
	var num strings.Builder
	for _, r := range rearranged {
		switch {
		case r >= '0' && r <= '9':
			num.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			v := int(r - 'A' + 10)
			num.WriteString(strconv.Itoa(v))
		default:
			return false
		}
	}
	rem := 0
	ns := num.String()
	for i := 0; i < len(ns); i++ {
		rem = (rem*10 + int(ns[i]-'0')) % 97
	}
	return rem == 1
}

func validEmailRFC5322Pragmatic(s string) bool {
	at := strings.LastIndex(s, "@")
	if at <= 0 || at >= len(s)-1 {
		return false
	}
	local, domain := s[:at], s[at+1:]
	if len(local) > 64 || len(domain) > 253 {
		return false
	}
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") ||
		strings.Contains(local, "..") {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") ||
		strings.Contains(domain, "..") {
		return false
	}
	for _, r := range local {
		if !isEmailLocalChar(r) {
			return false
		}
	}
	if !strings.Contains(domain, ".") {
		return false
	}
	for _, r := range domain {
		if !isEmailDomainChar(r) {
			return false
		}
	}
	return true
}

func isEmailLocalChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case strings.ContainsRune("!#$%&'*+/=?^_`{|}~.-", r):
		return true
	default:
		return false
	}
}

func isEmailDomainChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '-' || r == '.':
		return true
	default:
		return false
	}
}

func validTRMobile(s string) bool {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	n := b.String()
	switch len(n) {
	case 12:
		return strings.HasPrefix(n, "905")
	case 11:
		return strings.HasPrefix(n, "05")
	default:
		return false
	}
}

func isPrivateOrSpecialIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	if ip4[0] == 10 {
		return true
	}
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	if ip4[0] == 192 && ip4[1] == 168 {
		return true
	}
	if ip4[0] == 127 {
		return true
	}
	if ip4[0] == 169 && ip4[1] == 254 {
		return true
	}
	return false
}

// maskPAN preserves first 6 (BIN) and last 4 digits of a card number.
// Required by PCI-DSS v4.0 Requirement 3.4 for audit log display.
// Example: 4532015112830366 → 453201******0366
func maskPAN(pan string) string {
	d := digitsOnly(pan)
	if len(d) < 13 {
		return maskContent(pan) // fallback to generic masking
	}
	return d[:6] + strings.Repeat("*", len(d)-10) + d[len(d)-4:]
}

// maskContent replaces middle characters with asterisks for safe logging.
func maskContent(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	runes := []rune(s)
	for i := 2; i < len(runes)-2; i++ {
		if runes[i] != ' ' && runes[i] != '-' {
			runes[i] = '*'
		}
	}
	return string(runes)
}

// RedactContent replaces PII matches in content with placeholder tokens.
// It builds the redacted output from scratch by walking findings in ascending
// StartPos order, copying non-finding regions and inserting placeholders.
// This eliminates index-drift bugs caused by in-place replacement on a
// changing buffer where stale original offsets were used.
func RedactContent(content []byte, findings []Finding) []byte {
	var redactFindings []Finding
	for _, f := range findings {
		if f.Type != "pii" && f.Type != "custom" {
			continue
		}
		if f.StartPos < 0 || f.EndPos < 0 || f.StartPos >= f.EndPos || f.EndPos > len(content) {
			continue
		}
		redactFindings = append(redactFindings, f)
	}

	if len(redactFindings) == 0 {
		out := make([]byte, len(content))
		copy(out, content)
		return out
	}

	sort.Slice(redactFindings, func(i, j int) bool {
		return redactFindings[i].StartPos < redactFindings[j].StartPos
	})

	var buf bytes.Buffer
	pos := 0
	for _, f := range redactFindings {
		if f.StartPos > pos {
			buf.Write(content[pos:f.StartPos])
		}
		buf.WriteString("[" + f.Category + "_REDACTED]")
		pos = f.EndPos
	}
	if pos < len(content) {
		buf.Write(content[pos:])
	}
	return buf.Bytes()
}
