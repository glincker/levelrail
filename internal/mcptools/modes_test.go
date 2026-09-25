package mcptools

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listAll(t *testing.T, opts Options) []*mcp.Tool {
	t.Helper()
	server, _ := NewServerWithOptions(apiclient.NewClient("http://127.0.0.1:1", "t"), opts)
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(context.Background(), st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	var tools []*mcp.Tool
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}

func TestEveryToolClassified(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeFull})
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool.Name] = true
		m, ok := Lookup(tool.Name)
		if !ok || m.Class == ClassUnset || m.Group == "" {
			t.Errorf("tool %q has no explicit class and group in toolTable", tool.Name)
			continue
		}
		a := tool.Annotations
		if a == nil || a.OpenWorldHint == nil || a.Title == "" {
			t.Errorf("tool %q is missing annotations", tool.Name)
			continue
		}
		if a.ReadOnlyHint != (m.Class == ClassRead) {
			t.Errorf("tool %q: ReadOnlyHint %v does not match class %s", tool.Name, a.ReadOnlyHint, m.Class)
		}
		if m.Class != ClassRead && (a.DestructiveHint == nil || *a.DestructiveHint != (m.Class == ClassDestructive)) {
			t.Errorf("tool %q: DestructiveHint does not match class %s", tool.Name, m.Class)
		}
	}
	for name := range toolTable {
		if !seen[name] {
			t.Errorf("toolTable entry %q has no registered tool", name)
		}
	}
	t.Logf("registered tools: %d", len(tools))
}

func TestNamePrefixRules(t *testing.T) {
	rules := []struct {
		class    Class
		prefixes []string
	}{
		{ClassRead, []string{"get_", "list_", "explain_", "diagnose_"}},
		{ClassMutate, []string{"set_", "create_", "deploy_", "restart_", "rotate_", "approve_"}},
		{ClassDestructive, []string{"delete_", "clear_", "remove_", "destroy_", "drain_", "rollback_", "cancel_"}},
	}
	for name, m := range toolTable {
		for _, r := range rules {
			for _, p := range r.prefixes {
				if strings.HasPrefix(name, p) && m.Class != r.class {
					t.Errorf("%s must be %s, table says %s", name, r.class, m.Class)
				}
			}
		}
	}
}

func TestModesFilterRegistration(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want func(Meta) bool
	}{
		{"read-only", Options{Mode: ModeReadOnly}, func(m Meta) bool { return m.Class == ClassRead }},
		{"standard", Options{Mode: ModeStandard}, func(m Meta) bool { return m.Class != ClassDestructive }},
		{"default", Options{}, func(m Meta) bool { return m.Class != ClassDestructive }},
		{"full", Options{Mode: ModeFull}, func(Meta) bool { return true }},
		{"toolsets", Options{Mode: ModeFull, Toolsets: []string{"nodes", "models"}}, func(m Meta) bool { return m.Group == "nodes" || m.Group == "models" }},
		{"read-only toolset", Options{Mode: ModeReadOnly, Toolsets: []string{"models"}}, func(m Meta) bool { return m.Group == "models" && m.Class == ClassRead }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]bool{}
			for _, tool := range listAll(t, tc.opts) {
				got[tool.Name] = true
			}
			want := 0
			for name, m := range toolTable {
				if tc.want(m) {
					want++
					if !got[name] {
						t.Errorf("%s should be registered", name)
					}
				} else if got[name] {
					t.Errorf("%s must not be registered", name)
				}
			}
			if len(got) != want || want == 0 {
				t.Errorf("got %d tools, want %d", len(got), want)
			}
		})
	}
}

func TestSummaryCounts(t *testing.T) {
	_, sum := NewServerWithOptions(apiclient.NewClient("http://127.0.0.1:1", "t"), Options{Mode: ModeReadOnly})
	if sum.Mutate != 0 || sum.Destruct != 0 || sum.Total != sum.Read || sum.Read == 0 {
		t.Errorf("unexpected read-only summary %+v", sum)
	}
}

func TestParseModeAndToolsets(t *testing.T) {
	if m, err := ParseMode(""); err != nil || m != ModeStandard {
		t.Errorf("empty mode = %q, %v", m, err)
	}
	if _, err := ParseMode("yolo"); err == nil {
		t.Error("expected error for unknown mode")
	}
	if got, err := ParseToolsets(" Nodes, models ,"); err != nil || len(got) != 2 || got[0] != "nodes" {
		t.Errorf("toolsets = %v, %v", got, err)
	}
	if _, err := ParseToolsets("nope"); err == nil {
		t.Error("expected error for unknown toolset")
	}
}
