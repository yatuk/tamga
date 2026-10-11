package scanner

import (
	"context"
	"strconv"

	"github.com/yatuk/tamga/internal/scanner/linkscan"
)

// LinkScanner reports the links in a model's answer that carry data out,
// are not what they read as, or go to a host the operator denies. It is
// registered for answers only: a link in a request is the caller's own.
//
// The judging is done by the linkscan package, on the text alone. Nothing is
// resolved or fetched; a link is never followed to see where it leads.
//
// A finding's position is the link's address, so output_rules.redact_on can
// take the address out and leave the sentence around it.
type LinkScanner struct {
	// lists returns the operator's deny and allow lists, read on every scan
	// so that a policy reload applies to the next answer. It may be nil,
	// and may return nil.
	lists func() *linkscan.Lists
}

// NewLinkScanner creates the answer link scanner.
func NewLinkScanner(lists func() *linkscan.Lists) *LinkScanner {
	return &LinkScanner{lists: lists}
}

// LinkFindingType is the type of every finding this scanner makes. The
// categories are linkscan's.
const LinkFindingType = "malicious_link"

func (s *LinkScanner) Name() string { return LinkFindingType }

// linkGrades gives each category its severity and confidence. The form
// checks that rest on a threshold are less sure than the ones that rest on
// what the address is.
var linkGrades = map[string]struct {
	severity   string
	confidence float64
}{
	linkscan.DeniedHost:   {"high", 1.0},
	linkscan.Scheme:       {"high", 0.95},
	linkscan.Userinfo:     {"high", 0.9},
	linkscan.Homoglyph:    {"high", 0.85},
	linkscan.Exfiltration: {"high", 0.85},
	linkscan.TextMismatch: {"medium", 0.7},
	linkscan.IPHost:       {"medium", 0.6},
	linkscan.Port:         {"low", 0.5},
}

func (s *LinkScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	st := &scanStop{ctx: ctx, scanner: s.Name()}
	if st.stopped() {
		return nil, st.err
	}
	var lists *linkscan.Lists
	if s.lists != nil {
		lists = s.lists()
	}
	hits := linkscan.Scan(string(content), lists)
	if len(hits) == 0 {
		return nil, nil
	}
	findings := make([]Finding, 0, len(hits))
	for _, h := range hits {
		grade := linkGrades[h.Category]
		confidence := grade.confidence
		if h.Category == linkscan.Exfiltration && h.Kind != linkscan.KindImage {
			// A link has to be followed; an image is fetched by itself.
			confidence = 0.7
		}
		// The match is the host and never the address: the rest of an
		// address that carries data out is that data.
		match := h.Host
		if match == "" {
			match = h.Category
		}
		meta := map[string]string{"kind": h.Kind}
		if h.Payload > 0 {
			meta["chars"] = strconv.Itoa(h.Payload)
		}
		if h.Imitates != "" {
			// The protected name the host reads as. It is a name from the
			// policy or the built-in list, never text of the answer.
			meta["imitates"] = h.Imitates
		}
		findings = append(findings, Finding{
			Type:           LinkFindingType,
			Category:       h.Category,
			Severity:       grade.severity,
			Match:          match,
			StartPos:       h.Start,
			EndPos:         h.End,
			Confidence:     confidence,
			Metadata:       meta,
			ScannerVersion: ScannerVersions[LinkFindingType],
		})
		if len(findings) == MaxFindingsPerScan {
			break
		}
	}
	return findings, nil
}
