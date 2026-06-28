package normalize

import (
	"strings"
)

// ────────────────────────────────────────────────────────────────────────────
// Credential Formatting Normalization — 3.1 AWS Key Regex Hardening
// ────────────────────────────────────────────────────────────────────────────
//
// Attackers insert separators (spaces, dashes, plus signs) into credential
// strings to evade regex-based secret detection:
//
//	"AKIA IOSF ODNN 7EXAMPLE"     → evades \bAKIA[A-Z0-9]{16}\b
//	"ghp_ xxxx ... xxxx"          → evades \bghp_[A-Za-z0-9_]{36,}\b
//
// This file strips human-added separators from credential-like patterns,
// producing a clean variant that the secret scanner regex can match.
// ────────────────────────────────────────────────────────────────────────────

// credentialSpec defines a known credential type with its prefix, the set of
// valid body characters, and the minimum body length.
type credentialSpec struct {
	prefix    string
	keepChars func(rune) bool // which runes to keep in the body
	minBody   int             // minimum body length (exact for AWS, minimum for others)
	maxBody   int             // maximum body length (to bound the scan)
}

// gitHubPrefixes is the set of recognised GitHub PAT prefixes.
var gitHubPrefixes = []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}

// stripePrefixes is the set of recognised Stripe key prefixes.
var stripePrefixes = []string{"sk_live_", "sk_test_", "pk_live_", "pk_test_"}

// credentialSpecs is the canonical set of credential types that need
// separator-stripping. AWS is handled by tryAWS (multi-prefix); GitHub
// and Stripe have their own multi-prefix handlers. This list covers the
// single-prefix credential types.
var credentialSpecs = []credentialSpec{
	// Anthropic: sk-ant- + ≥20 alphanumeric, dash, or underscore chars.
	{
		prefix: "sk-ant-",
		keepChars: func(r rune) bool {
			return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_'
		},
		minBody: 20,
		maxBody: 255,
	},

	// OpenAI: sk- (but NOT sk-ant-) + ≥20 alphanumeric, dash, or underscore.
	{
		prefix: "sk-",
		keepChars: func(r rune) bool {
			return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_'
		},
		minBody: 20,
		maxBody: 255,
	},
}

// validGitHubChar reports whether r is valid in a GitHub PAT body.
func validGitHubChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') || r == '_'
}

// validStripeChar reports whether r is valid in a Stripe key body.
func validStripeChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// awsPrefixes is the set of recognised AWS access key ID prefixes.
var awsPrefixes = []string{
	"AKIA", "ASIA", "AGPA", "AIDA", "AROA", "AIPA", "ANPA", "ANVA",
}

// isSeparator reports whether r is a separator commonly inserted by humans
// for readability (spaces, dashes, plus signs).
func isSeparator(r rune) bool {
	return r == ' ' || r == '-' || r == '+'
}

// isAWSChar reports whether r is valid in an AWS access key body.
func isAWSChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// normalizeCredentials strips separators from credential-like patterns in s.
// It finds known credential prefixes, then scans forward collecting valid
// body characters while skipping common separators. If enough valid chars
// are collected, the original span is replaced with the clean key.
func normalizeCredentials(s string) string {
	if len(s) < 20 {
		return s // shortest credential (AWS) is 20 chars
	}

	var out strings.Builder
	out.Grow(len(s))

	runes := []rune(s)
	i := 0
	for i < len(runes) {
		// ── Try AWS prefixes (multi-prefix, 4-char prefix + exactly 16 body) ──
		if awsPrefix, awsEnd := tryAWS(runes, i); awsPrefix != "" {
			out.WriteString(awsPrefix)
			for _, r := range runes[i+len([]rune(awsPrefix)) : awsEnd] {
				if isAWSChar(r) {
					out.WriteRune(r)
				}
			}
			i = awsEnd
			continue
		}

		// ── Try GitHub prefixes (gh[opusr]_ + ≥36 alphanumeric/underscore) ──
		if gitPrefix, gitEnd := tryMultiPrefix(runes, i, gitHubPrefixes, validGitHubChar, 36, 255); gitPrefix != "" {
			out.WriteString(gitPrefix)
			for _, r := range runes[i+len([]rune(gitPrefix)) : gitEnd] {
				if validGitHubChar(r) {
					out.WriteRune(r)
				}
			}
			i = gitEnd
			continue
		}

		// ── Try Stripe prefixes (sk/pk_live/test_ + ≥24 alphanumeric) ──
		if stripePrefix, stripeEnd := tryMultiPrefix(runes, i, stripePrefixes, validStripeChar, 24, 255); stripePrefix != "" {
			out.WriteString(stripePrefix)
			for _, r := range runes[i+len([]rune(stripePrefix)) : stripeEnd] {
				if validStripeChar(r) {
					out.WriteRune(r)
				}
			}
			i = stripeEnd
			continue
		}

		// ── Try generic credential specs (Anthropic, OpenAI) ──
		matched := false
		for _, spec := range credentialSpecs {
			if spec.prefix == "" {
				continue
			}
			if bodyLen, end := tryCredential(runes, i, spec); bodyLen >= spec.minBody {
				out.WriteString(spec.prefix)
				for _, r := range runes[i+len([]rune(spec.prefix)) : end] {
					if spec.keepChars(r) {
						out.WriteRune(r)
					}
				}
				i = end
				matched = true
				break
			}
		}
		if !matched {
			out.WriteRune(runes[i])
			i++
		}
	}
	return out.String()
}

// tryAWS checks whether runes[i:] starts with a known AWS access key prefix
// (including A3T[X] where X is A-Z0-9) followed by, after stripping separators,
// exactly 16 uppercase alphanumeric chars. Returns the matched prefix and the
// end position in runes (past the last consumed character).
func tryAWS(runes []rune, i int) (prefix string, end int) {
	remaining := string(runes[i:])

	// Collect candidate prefixes: the fixed 4-char set plus A3T + one [A-Z0-9].
	candidates := make([]string, 0, len(awsPrefixes)+1)
	candidates = append(candidates, awsPrefixes...)
	// A3T[A-Z0-9]: check that we have at least 5 runes and the 4th is [A-Z0-9].
	if len(runes)-i >= 5 && strings.HasPrefix(remaining, "A3T") {
		fifth := runes[i+3]
		if (fifth >= 'A' && fifth <= 'Z') || (fifth >= '0' && fifth <= '9') {
			candidates = append(candidates, string(runes[i:i+4]))
		}
	}

	for _, p := range candidates {
		if !strings.HasPrefix(remaining, p) {
			continue
		}
		prefixRunes := []rune(p)
		j := i + len(prefixRunes)
		collected := 0
		scanEnd := j
		for j < len(runes) && collected < 16 {
			r := runes[j]
			if isAWSChar(r) {
				collected++
				scanEnd = j + 1
			} else if isSeparator(r) {
				// skip
			} else {
				break
			}
			j++
		}
		if collected == 16 {
			return p, scanEnd
		}
	}
	return "", i
}

// tryMultiPrefix checks whether runes[i:] starts with any of the given prefixes
// followed by (after stripping separators) at least minBody valid chars.
func tryMultiPrefix(runes []rune, i int, prefixes []string, keep func(rune) bool, minBody, maxBody int) (prefix string, end int) {
	remaining := string(runes[i:])
	for _, p := range prefixes {
		if !strings.HasPrefix(remaining, p) {
			continue
		}
		prefixRunes := []rune(p)
		j := i + len(prefixRunes)
		collected := 0
		scanEnd := j
		for j < len(runes) && collected < maxBody {
			r := runes[j]
			if keep(r) {
				collected++
				scanEnd = j + 1
			} else if isSeparator(r) {
				// skip
			} else {
				break
			}
			j++
		}
		if collected >= minBody {
			return p, scanEnd
		}
	}
	return "", i
}

// tryCredential checks whether runes[i:] matches a generic credential spec:
// the literal prefix, followed by (after stripping separators) at least
// minBody valid body characters.
func tryCredential(runes []rune, i int, spec credentialSpec) (bodyLen int, end int) {
	prefixRunes := []rune(spec.prefix)
	if i+len(prefixRunes) > len(runes) {
		return 0, i
	}
	// Check prefix match (case-sensitive).
	remaining := string(runes[i:])
	if !strings.HasPrefix(remaining, spec.prefix) {
		return 0, i
	}

	j := i + len(prefixRunes)
	collected := 0
	scanEnd := j
	// For sk- prefix, exclude sk-ant- (handled by Anthropic spec).
	if spec.prefix == "sk-" && j+3 <= len(runes) {
		if string(runes[j:j+4]) == "ant-" {
			return 0, i // let the Anthropic spec handle it
		}
	}

	for j < len(runes) && collected < spec.maxBody {
		r := runes[j]
		if spec.keepChars(r) {
			collected++
			scanEnd = j + 1
		} else if isSeparator(r) {
			// skip
		} else {
			break
		}
		j++
	}
	if collected >= spec.minBody {
		return collected, scanEnd
	}
	return 0, i
}
