// Command gen-runtime-settings-docs writes docs/runtime-settings.md and the
// generated env seed block in docs/environment-variables.md from the real
// runtime settings registry, so the docs cannot drift from the code.
//
// Usage (from the repo root): go run ./scripts/gen-runtime-settings-docs
// It exits non-zero when a registry entry has no description.
package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	// The api package registers its own settings in init, so import it for the full registry.
	_ "github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	pagePath    = "docs/runtime-settings.md"
	envPath     = "docs/environment-variables.md"
	markerStart = "<!-- runtime-settings:start -->"
	markerEnd   = "<!-- runtime-settings:end -->"
)

var categoryOrder = []runtimesettings.Category{
	runtimesettings.CategorySecurity, runtimesettings.CategoryNetworking, runtimesettings.CategoryDeploys,
	runtimesettings.CategoryObservability, runtimesettings.CategoryRetention, runtimesettings.CategoryExperimental,
}

var categoryTitle = map[runtimesettings.Category]string{
	runtimesettings.CategorySecurity:      "Security",
	runtimesettings.CategoryNetworking:    "Networking",
	runtimesettings.CategoryDeploys:       "Deploys",
	runtimesettings.CategoryObservability: "Observability",
	runtimesettings.CategoryRetention:     "Data retention",
	runtimesettings.CategoryExperimental:  "Experimental",
}

func main() {
	defs := runtimesettings.Definitions()
	var missing []string
	for _, d := range defs {
		if strings.TrimSpace(d.Description) == "" {
			missing = append(missing, d.Key)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "runtime settings without a description: %s\n", strings.Join(missing, ", "))
		os.Exit(1)
	}
	if err := os.WriteFile(pagePath, []byte(renderPage(defs)), 0o644); err != nil { //nolint:gosec // docs file
		fatal(err)
	}
	if err := patchEnvDoc(defs); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen-runtime-settings-docs:", err)
	os.Exit(1)
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteRune('-')
		}
	}
	return b.String()
}

func byCategory(defs []runtimesettings.Definition) map[runtimesettings.Category][]runtimesettings.Definition {
	m := map[runtimesettings.Category][]runtimesettings.Definition{}
	for _, d := range defs {
		m[d.Category] = append(m[d.Category], d)
	}
	for c := range m {
		list := m[c]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	}
	return m
}

func orderedCategories(m map[runtimesettings.Category][]runtimesettings.Definition) []runtimesettings.Category {
	var out []runtimesettings.Category
	seen := map[runtimesettings.Category]bool{}
	for _, c := range categoryOrder {
		if len(m[c]) > 0 {
			out = append(out, c)
			seen[c] = true
		}
	}
	var rest []runtimesettings.Category
	for c := range m {
		if !seen[c] {
			rest = append(rest, c)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i] < rest[j] })
	return append(out, rest...)
}

func titleOf(c runtimesettings.Category) string {
	if t, ok := categoryTitle[c]; ok {
		return t
	}
	s := string(c)
	return strings.ToUpper(s[:1]) + s[1:]
}

func esc(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "|", `\|`) }

func defaultText(d runtimesettings.Definition) string {
	if d.Default == "" {
		return "empty"
	}
	return "`" + strings.ReplaceAll(d.Default, "|", `\|`) + "`"
}

// rangeText probes the validators with out-of-range values, since they are opaque funcs.
func rangeText(d runtimesettings.Definition) string {
	var parts []string
	if len(d.AllowedInts) > 0 {
		var xs []string
		for _, n := range d.AllowedInts {
			xs = append(xs, strconv.Itoa(n))
		}
		parts = append(parts, "one of "+strings.Join(xs, ", "))
	}
	if len(d.AllowedStrings) > 0 {
		parts = append(parts, "one of "+strings.Join(d.AllowedStrings, ", "))
	}
	if d.CheckValue != nil {
		probes := []string{"-1", "0", "1ns"}
		for _, p := range probes {
			if err := d.CheckValue(p); err != nil {
				parts = append(parts, err.Error())
				break
			}
		}
	}
	if d.Kind == runtimesettings.KindStringList {
		parts = append(parts, "colon separated list")
	}
	if len(parts) == 0 {
		return "any valid " + string(d.Kind)
	}
	return esc(strings.Join(parts, "; "))
}

func envText(d runtimesettings.Definition) string {
	if d.Env == "" {
		return "none"
	}
	return "`" + d.Env + "`"
}

func liveText(d runtimesettings.Definition) string {
	if d.Apply == runtimesettings.ApplyLive {
		return "yes"
	}
	return "no, restart"
}

func renderPage(defs []runtimesettings.Definition) string {
	m := byCategory(defs)
	var b strings.Builder
	b.WriteString("---\ntitle: \"Runtime settings\"\n")
	b.WriteString("description: Every runtime setting with its type, default, range, environment seed variable and where to change it.\n")
	b.WriteString("\"og:url\": \"https://docs.levelrail.com/runtime-settings\"\n---\n\n")
	b.WriteString("{/* Generated by scripts/gen-runtime-settings-docs from internal/runtimesettings. Do not edit by hand. */}\n\n")
	b.WriteString("Runtime settings are the controls operators change in production. A stored value always wins, the environment variable only seeds the default while nothing is stored, and a setting marked live applies without a restart. ")
	fmt.Fprintf(&b, "There are %d settings.\n\n", len(defs))
	b.WriteString("Change one in the dashboard under Settings > Runtime, or with the CLI:\n\n```bash\nlevelrail-cli settings runtime list\nlevelrail-cli settings runtime get <key>\nlevelrail-cli settings runtime set <key> <value>\nlevelrail-cli settings runtime reset <key>\n```\n\n")
	b.WriteString("The environment variables that seed these defaults are also listed on [Environment variables](/environment-variables#runtime-settings).\n")
	for _, c := range orderedCategories(m) {
		fmt.Fprintf(&b, "\n## %s\n\n", titleOf(c))
		b.WriteString("| Key | Type | Default | Range | Env seed | Live | Description | CLI | Dashboard |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, d := range m[c] {
			desc := esc(d.Description)
			if d.Risk == runtimesettings.RiskWeakensSecurity {
				desc += " Weakening it asks for confirmation."
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s | `levelrail-cli settings runtime get\\|set %s` | Settings > Runtime, %s card |\n",
				d.Key, d.Kind, defaultText(d), rangeText(d), envText(d), liveText(d), desc, d.Key, titleOf(c))
		}
	}
	return b.String()
}

func envBlock(defs []runtimesettings.Definition) string {
	m := byCategory(defs)
	var b strings.Builder
	b.WriteString(markerStart + "\n")
	b.WriteString("{/* Generated by scripts/gen-runtime-settings-docs. Do not edit by hand. */}\n")
	b.WriteString("These variables only seed the default of a [runtime setting](/runtime-settings). A stored value wins and applies without a restart unless the setting says otherwise.\n\n")
	b.WriteString("| Variable | Setting | Default |\n| --- | --- | --- |\n")
	for _, c := range orderedCategories(m) {
		for _, d := range m[c] {
			if d.Env == "" {
				continue
			}
			fmt.Fprintf(&b, "| `%s` | [`%s`](/runtime-settings#%s) | %s |\n", d.Env, d.Key, slug(titleOf(c)), defaultText(d))
		}
	}
	b.WriteString(markerEnd)
	return b.String()
}

func patchEnvDoc(defs []runtimesettings.Definition) error {
	raw, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}
	s := string(raw)
	i, j := strings.Index(s, markerStart), strings.Index(s, markerEnd)
	if i < 0 || j < i {
		return fmt.Errorf("%s is missing the %s / %s markers", envPath, markerStart, markerEnd)
	}
	out := s[:i] + envBlock(defs) + s[j+len(markerEnd):]
	return os.WriteFile(envPath, []byte(out), 0o644) //nolint:gosec // docs file
}
