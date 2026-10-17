package scanner

// ToolDestinationFindingType is the type of a finding about a place that a
// tool call in an answer sends data to and that output_rules.tool_destinations
// does not allow (policy.ToolDestinationRules). The proxy makes these
// findings from the tool name and the arguments; no scanner of a registry
// does. The match is the host, the position the address in the arguments.
const ToolDestinationFindingType = "tool_destination"

// DestinationNotAllowed is the category of every tool_destination finding.
const DestinationNotAllowed = "destination_not_allowed"
