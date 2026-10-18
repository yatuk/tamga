package scanner

// ToolProvenanceFindingType is the type of a finding about a tool call in
// an answer: a call of a tool that agents.provenance names high-risk, made
// when the conversation the request carried held tool content the policy
// does not trust (policy.ProvenanceSettings). The proxy makes these
// findings from the tool name and the request's own findings; no scanner
// of a registry does. The match is the tool's name. The metadata says which
// tool content made the request untrusted, by its path and finding type,
// never by its text.
const ToolProvenanceFindingType = "tool_provenance"

// TaintedToolCall is the category of every tool_provenance finding.
const TaintedToolCall = "tainted_tool_call"
