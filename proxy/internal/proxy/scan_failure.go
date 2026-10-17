package proxy

import "github.com/yatuk/tamga/internal/extract"

// carriesToolContent reports whether a request holds tool results or tool
// definitions: text that came from outside the conversation, or a tool the
// model may call. It is agent traffic. A policy that leaves scan.on_error
// unset fails such a request closed when a scan fails (Policy.BlockOnScanError).
func carriesToolContent(segs []extract.Segment) bool {
	for _, s := range segs {
		if s.Role == extract.RoleTool || s.Role == extract.RoleToolDefinition {
			return true
		}
	}
	return false
}
