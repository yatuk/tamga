package scanner

import (
	pb "github.com/yatuk/tamga/proto/scanner/v1"
)

// FindingsToProto converts the internal Go Finding slice to proto messages.
func FindingsToProto(fs []Finding) []*pb.Finding {
	if len(fs) == 0 {
		return nil
	}
	out := make([]*pb.Finding, 0, len(fs))
	for _, f := range fs {
		pf := &pb.Finding{
			Type:           f.Type,
			Severity:       f.Severity,
			Match:          f.Match,
			Category:       f.Category,
			StartPos:       int32(f.StartPos),
			EndPos:         int32(f.EndPos),
			Confidence:     f.Confidence,
			ActionTaken:    f.ActionTaken,
			Metadata:       f.Metadata,
			ScannerVersion: f.ScannerVersion,
			DatasetVersion: f.DatasetVersion,
		}
		// ConfidenceScore: embed as metadata if present.
		if f.ConfidenceScore != nil {
			if pf.Metadata == nil {
				pf.Metadata = make(map[string]string)
			}
			pf.Metadata["confidence_score_total"] = f.ConfidenceScore.Reasoning
		}
		out = append(out, pf)
	}
	return out
}

// ProtoToFindings converts proto Finding messages to the internal Go type.
func ProtoToFindings(pfs []*pb.Finding) []Finding {
	if len(pfs) == 0 {
		return nil
	}
	out := make([]Finding, 0, len(pfs))
	for _, pf := range pfs {
		out = append(out, Finding{
			Type:           pf.Type,
			Severity:       pf.Severity,
			Match:          pf.Match,
			Category:       pf.Category,
			StartPos:       int(pf.StartPos),
			EndPos:         int(pf.EndPos),
			Confidence:     pf.Confidence,
			ActionTaken:    pf.ActionTaken,
			Metadata:       pf.Metadata,
			ScannerVersion: pf.ScannerVersion,
			DatasetVersion: pf.DatasetVersion,
		})
	}
	return out
}
