package ai

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func boolPtr(b bool) *bool { return &b }

func TestTraitsFromMCP(t *testing.T) {
	tests := []struct {
		name string
		ann  *mcp.ToolAnnotations
		meta map[string]any
		want ToolTraits
	}{
		{"nil annotations are unknown", nil, nil, ToolTraits{}},
		{"read only closed world", &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}, nil, ToolTraits{Known: true, ReadOnly: true}},
		{"open world defaults true", &mcp.ToolAnnotations{ReadOnlyHint: true}, nil, ToolTraits{Known: true, ReadOnly: true, OpenWorld: true}},
		{"destructive default when unset", &mcp.ToolAnnotations{OpenWorldHint: boolPtr(false)}, nil, ToolTraits{Known: true, Destructive: true}},
		{"mutating non destructive", &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)}, nil, ToolTraits{Known: true}},
		{"meta flags", &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}, map[string]any{mcptools.MetaSensitiveKey: true, mcptools.MetaUntrustedKey: true}, ToolTraits{Known: true, ReadOnly: true, Sensitive: true, UntrustedOutput: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TraitsFromMCP(tt.ann, tt.meta); got != tt.want {
				t.Errorf("TraitsFromMCP() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRequiresConfirmation(t *testing.T) {
	tests := []struct {
		name    string
		traits  ToolTraits
		tainted bool
		want    bool
	}{
		{"unknown always confirms", ToolTraits{}, false, true},
		{"mutating always confirms", ToolTraits{Known: true}, false, true},
		{"destructive always confirms", ToolTraits{Known: true, Destructive: true}, false, true},
		{"clean read runs", ToolTraits{Known: true, ReadOnly: true}, false, false},
		{"clean read runs while tainted", ToolTraits{Known: true, ReadOnly: true}, true, false},
		{"outbound read runs when clean", ToolTraits{Known: true, ReadOnly: true, OpenWorld: true}, false, false},
		{"outbound read confirms when tainted", ToolTraits{Known: true, ReadOnly: true, OpenWorld: true}, true, true},
		{"sensitive read confirms when tainted", ToolTraits{Known: true, ReadOnly: true, Sensitive: true}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.traits.RequiresConfirmation(tt.tainted); got != tt.want {
				t.Errorf("RequiresConfirmation(%v) = %v, want %v", tt.tainted, got, tt.want)
			}
		})
	}
}

// TestRealToolSurfaceGate drives the gate over every tool the platform
// actually registers, through the same in-process MCP path the assistant
// uses.
func TestRealToolSurfaceGate(t *testing.T) {
	ctx := context.Background()
	tc, err := NewToolCaller(ctx, apiclient.NewClient("http://127.0.0.1:1", "t"))
	if err != nil {
		t.Fatalf("NewToolCaller: %v", err)
	}
	t.Cleanup(func() { _ = tc.Close() })
	specs, err := tc.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("no tools listed")
	}
	for _, s := range specs {
		meta, ok := mcptools.Lookup(s.Name)
		if !ok {
			t.Errorf("%s is not classified", s.Name)
			continue
		}
		if !s.Traits.Known {
			t.Errorf("%s reached the assistant without annotations", s.Name)
		}
		if meta.Class != mcptools.ClassRead {
			if !s.Traits.RequiresConfirmation(false) {
				t.Errorf("%s (%s) must always require confirmation", s.Name, meta.Class)
			}
			continue
		}
		if s.Traits.RequiresConfirmation(false) {
			t.Errorf("read tool %s must not need confirmation while clean", s.Name)
		}
		if (meta.Outbound() || meta.Sensitive()) != s.Traits.RequiresConfirmation(true) {
			t.Errorf("read tool %s: tainted gate = %v, want outbound/sensitive %v", s.Name, s.Traits.RequiresConfirmation(true), meta.Outbound() || meta.Sensitive())
		}
		if meta.Untrusted() != s.Traits.UntrustedOutput {
			t.Errorf("%s: untrusted flag lost across MCP", s.Name)
		}
	}
}
