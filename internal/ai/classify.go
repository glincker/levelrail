package ai

import "strings"

// IsReadOnly reports whether an MCP tool is safe to auto-execute as part
// of the model's own turn, versus a mutating tool that must always pause
// for a human confirmation click (POST .../confirmations/{id}) before it
// runs, regardless of what the model requests.
//
// This is a real security boundary, not a minor detail: the MCP go-sdk
// (github.com/modelcontextprotocol/go-sdk/mcp, checked against
// cmd/levelrail-mcp's/internal/mcptools' own tool registrations) carries
// no destructive/read-only annotation on any tool registered there
// today, so classification falls back to name pattern, the same
// heuristic the MCP spec itself documents as a fallback when a server
// omits tool annotations: a "list_", "get_", or "diagnose_" prefix names
// a read (a list, a get, or diagnose.go's read-only failure analysis);
// every other prefix (deploy_, rollback_, restart_, clone_, prune_,
// promote_, check_, test_, sweep_, ...) is treated as mutating and
// always pauses, fail-safe by construction: an unrecognized or
// ambiguous name defaults to requiring confirmation, never to
// auto-executing.
func IsReadOnly(toolName string) bool {
	prefix, _, found := strings.Cut(toolName, "_")
	if !found {
		return false
	}
	switch prefix {
	case "list", "get", "diagnose":
		return true
	default:
		return false
	}
}
