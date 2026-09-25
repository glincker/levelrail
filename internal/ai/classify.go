package ai

import (
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolTraits is what the assistant knows about a tool from its MCP
// annotations. The zero value (Known false) is a tool with no
// annotations, which always requires confirmation.
type ToolTraits struct {
	Known           bool
	ReadOnly        bool
	Destructive     bool
	OpenWorld       bool
	Sensitive       bool
	UntrustedOutput bool
}

// TraitsFromMCP derives ToolTraits from a tool's annotations and _meta.
// Absent annotations mean unknown; an absent OpenWorldHint means open
// world, per the MCP default.
func TraitsFromMCP(a *mcp.ToolAnnotations, meta map[string]any) ToolTraits {
	if a == nil {
		return ToolTraits{}
	}
	t := ToolTraits{Known: true, ReadOnly: a.ReadOnlyHint, OpenWorld: a.OpenWorldHint == nil || *a.OpenWorldHint}
	t.Destructive = !a.ReadOnlyHint && (a.DestructiveHint == nil || *a.DestructiveHint)
	t.Sensitive, _ = meta[mcptools.MetaSensitiveKey].(bool)
	t.UntrustedOutput, _ = meta[mcptools.MetaUntrustedKey].(bool)
	return t
}

// RequiresConfirmation reports whether a call must pause for a human
// click. Anything that is not a known read-only tool always pauses. Once
// the conversation has ingested untrusted text (tainted), a read-only
// tool that is outbound or touches secrets pauses too.
func (t ToolTraits) RequiresConfirmation(tainted bool) bool {
	if !t.Known || !t.ReadOnly {
		return true
	}
	return tainted && (t.OpenWorld || t.Sensitive)
}

// IngestsUntrusted reports whether running the tool puts untrusted text
// in the conversation. An unknown tool is assumed to.
func (t ToolTraits) IngestsUntrusted() bool {
	return !t.Known || t.UntrustedOutput
}

// conversationTainted reports whether any resolved tool call in rows
// returned untrusted text. Pending and rejected calls returned nothing.
func conversationTainted(rows []store.AIChatMessage, traits map[string]ToolTraits) bool {
	for _, row := range rows {
		for _, tc := range row.ToolCalls {
			if tc.Status == store.AIChatConfirmationPending || tc.Status == store.AIChatConfirmationRejected {
				continue
			}
			if traits[tc.Name].IngestsUntrusted() {
				return true
			}
		}
	}
	return false
}
