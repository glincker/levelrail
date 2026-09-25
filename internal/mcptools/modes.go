package mcptools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Mode bounds which tool classes a server registers. The API token's own
// abilities still apply on top: a mode only shrinks what the model sees.
type Mode string

const (
	// ModeReadOnly registers read tools only.
	ModeReadOnly Mode = "read-only"
	// ModeStandard registers read and mutating tools, not destructive ones.
	ModeStandard Mode = "standard"
	// ModeFull registers every tool.
	ModeFull Mode = "full"

	// EnvMode names the env var that selects the Mode.
	EnvMode = "APP_MCP_MODE"
	// EnvToolsets names the env var that selects the toolsets.
	EnvToolsets = "APP_MCP_TOOLSETS"
)

// ParseMode parses a mode name, defaulting an empty value to standard.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return ModeStandard, nil
	case ModeReadOnly, ModeStandard, ModeFull:
		return m, nil
	default:
		return "", fmt.Errorf("unknown mcp mode %q, want %q, %q or %q", s, ModeReadOnly, ModeStandard, ModeFull)
	}
}

// ParseToolsets parses a comma separated toolset list; empty means all.
func ParseToolsets(s string) ([]string, error) {
	valid := map[string]struct{}{}
	for _, g := range Groups() {
		valid[g] = struct{}{}
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		g := strings.ToLower(strings.TrimSpace(part))
		if g == "" {
			continue
		}
		if _, ok := valid[g]; !ok {
			return nil, fmt.Errorf("unknown mcp toolset %q, want one of %s", g, strings.Join(Groups(), ", "))
		}
		out = append(out, g)
	}
	return out, nil
}

// Options selects the tools a server exposes.
type Options struct {
	Mode     Mode
	Toolsets []string
}

// Summary reports what a server exposes.
type Summary struct {
	Mode     Mode
	Toolsets []string
	Total    int
	Read     int
	Mutate   int
	Destruct int
}

func (o Options) allows(m Meta) bool {
	switch o.Mode {
	case ModeReadOnly:
		if m.Class != ClassRead {
			return false
		}
	case ModeFull:
	default:
		if m.Class == ClassDestructive {
			return false
		}
	}
	if len(o.Toolsets) == 0 {
		return true
	}
	for _, g := range o.Toolsets {
		if g == m.Group {
			return true
		}
	}
	return false
}

func applyOptions(server *mcp.Server, opts Options) Summary {
	sum := Summary{Mode: opts.Mode, Toolsets: opts.Toolsets}
	var hidden []string
	for name, m := range toolTable {
		if !opts.allows(m) {
			hidden = append(hidden, name)
			continue
		}
		sum.Total++
		switch m.Class {
		case ClassRead:
			sum.Read++
		case ClassMutate:
			sum.Mutate++
		case ClassDestructive:
			sum.Destruct++
		}
	}
	sort.Strings(hidden)
	server.RemoveTools(hidden...)
	return sum
}
