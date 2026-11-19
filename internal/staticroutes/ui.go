package staticroutes

import (
	"fmt"
	"sort"
	"strings"
)

// SourceUI labels rules edited in the dashboard; they rank before app.yaml and the files.
const SourceUI = "ui"

// UIRedirect is one dashboard redirect rule.
type UIRedirect struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status int    `json:"status,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

// UIHeader is one dashboard header block.
type UIHeader struct {
	Path   string            `json:"path"`
	Set    map[string]string `json:"set,omitempty"`
	Remove []string          `json:"remove,omitempty"`
}

// OptionOverrides replace the serving options a deploy compiled; nil or empty keeps them.
type OptionOverrides struct {
	SPA           *bool
	CleanURLs     *bool
	TrailingSlash string
	NotFound      string
}

// LineProblem is one validation failure of a dashboard rule.
type LineProblem struct {
	Kind    string `json:"kind"`
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// SourceKind maps a rule's Source label to file, app.yaml or ui.
func SourceKind(source string) string {
	switch {
	case strings.HasPrefix(source, SourceUI):
		return "ui"
	case strings.HasPrefix(source, SourceSpec):
		return "app.yaml"
	default:
		return "file"
	}
}

// CompileUI validates dashboard rules, returning prepared entries or every problem found.
func CompileUI(redirects []UIRedirect, headers []UIHeader, l Limits) ([]Rule, []HeaderBlock, []LineProblem) {
	var probs []LineProblem
	rules := make([]Rule, 0, len(redirects))
	for i, r := range redirects {
		rule := Rule{Source: fmt.Sprintf("%s routes[%d]", SourceUI, i), From: r.From, To: r.To, Status: r.Status, Force: r.Force}
		if err := ValidateRule(&rule, l); err != nil {
			probs = append(probs, LineProblem{Kind: "redirect", Index: i, Message: err.Error()})
			continue
		}
		rules = append(rules, rule)
	}
	blocks := make([]HeaderBlock, 0, len(headers))
	for i, h := range headers {
		b := HeaderBlock{Source: fmt.Sprintf("%s headers[%d]", SourceUI, i), Path: h.Path, Remove: h.Remove}
		names := make([]string, 0, len(h.Set))
		for k := range h.Set {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			b.Set = append(b.Set, HeaderKV{Name: k, Value: h.Set[k]})
		}
		if err := ValidateHeaderBlock(&b, l); err != nil {
			probs = append(probs, LineProblem{Kind: "header", Index: i, Message: err.Error()})
			continue
		}
		blocks = append(blocks, b)
	}
	if len(rules)+len(redirects)-len(rules) > l.MaxRules {
		probs = append(probs, LineProblem{Kind: "redirect", Index: -1, Message: fmt.Sprintf("%d rules is over the cap of %d (%s)", len(redirects), l.MaxRules, EnvMaxRules)})
	}
	if len(headers) > l.MaxHeaderBlocks {
		probs = append(probs, LineProblem{Kind: "header", Index: -1, Message: fmt.Sprintf("%d header blocks is over the cap of %d (%s)", len(headers), l.MaxHeaderBlocks, EnvMaxHeaderRules)})
	}
	return rules, blocks, probs
}

// ApplyUI drops earlier dashboard entries from rs, puts the given ones first and
// applies the option overrides. Running it twice with the same input is a no-op.
func (rs *Ruleset) ApplyUI(rules []Rule, blocks []HeaderBlock, o OptionOverrides) error {
	keptRules := make([]Rule, 0, len(rs.Rules)+len(rules))
	keptRules = append(keptRules, rules...)
	for _, r := range rs.Rules {
		if SourceKind(r.Source) != "ui" {
			keptRules = append(keptRules, r)
		}
	}
	keptBlocks := make([]HeaderBlock, 0, len(rs.Headers)+len(blocks))
	keptBlocks = append(keptBlocks, blocks...)
	for _, b := range rs.Headers {
		if SourceKind(b.Source) != "ui" {
			keptBlocks = append(keptBlocks, b)
		}
	}
	rs.Rules, rs.Headers = keptRules, keptBlocks
	if rs.BaseOptions == nil {
		base := rs.Options
		rs.BaseOptions = &base
	}
	rs.Options = *rs.BaseOptions
	if o.SPA != nil {
		rs.Options.SPA = *o.SPA
	}
	if o.CleanURLs != nil {
		rs.Options.CleanURLs = *o.CleanURLs
	}
	if o.TrailingSlash != "" {
		rs.Options.TrailingSlash = o.TrailingSlash
	}
	if o.NotFound != "" {
		rs.Options.NotFound = o.NotFound
	}
	return validateOptions(&rs.Options)
}

// ValidateOptions checks the serving switches.
func ValidateOptions(o *Options) error { return validateOptions(o) }
