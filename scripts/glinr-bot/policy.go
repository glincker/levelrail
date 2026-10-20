package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Action names a policy rule can ask for.
const (
	ActionComment   = "comment"
	ActionApprove   = "approve"
	ActionAutomerge = "automerge"
)

// Modes. Shadow only ever comments, so a new policy can run beside humans first.
const (
	ModeShadow  = "shadow"
	ModeEnforce = "enforce"
)

// Policy is the repo's .github/glinr-bot.yml, always read from the base branch.
type Policy struct {
	Version     int      `yaml:"version"`
	Mode        string   `yaml:"mode"`
	DenyPaths   []string `yaml:"deny_paths"`
	BlockLabels []string `yaml:"block_labels"`
	Rules       []Rule   `yaml:"rules"`
}

// Rule is one set of conditions and the actions earned when all hold.
type Rule struct {
	Name            string   `yaml:"name"`
	Authors         []string `yaml:"authors"`
	TitlePrefix     string   `yaml:"title_prefix"`
	PathsOnly       []string `yaml:"paths_only"`
	MaxFiles        int      `yaml:"max_files"`
	MaxChangedLines int      `yaml:"max_changed_lines"`
	UpdateTypes     []string `yaml:"update_types"`
	Actions         []string `yaml:"actions"`
}

// PR is the slice of a pull request the policy looks at.
type PR struct {
	Author       string
	Title        string
	Draft        bool
	Fork         bool
	Labels       []string
	Files        []string
	ChangedLines int
	SelfAuthored bool
}

// Gate is one yes or no check, shown in the PR comment.
type Gate struct {
	Name   string
	Pass   bool
	Detail string
}

// Verdict is the outcome for one PR.
type Verdict struct {
	Rule    string
	Gates   []Gate
	Actions []string
	Reason  string
}

// ParsePolicy decodes and validates a policy file.
func ParsePolicy(b []byte) (Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("parse policy: %w", err)
	}
	if p.Version != 1 {
		return p, fmt.Errorf("policy version must be 1, got %d", p.Version)
	}
	if p.Mode == "" {
		p.Mode = ModeShadow
	}
	if p.Mode != ModeShadow && p.Mode != ModeEnforce {
		return p, fmt.Errorf("mode must be %s or %s", ModeShadow, ModeEnforce)
	}
	for i, r := range p.Rules {
		if r.Name == "" {
			return p, fmt.Errorf("rule %d has no name", i)
		}
		for _, a := range r.Actions {
			if a != ActionComment && a != ActionApprove && a != ActionAutomerge {
				return p, fmt.Errorf("rule %q: unknown action %q", r.Name, a)
			}
		}
	}
	return p, nil
}

// globMatch matches a path against a pattern where * stays inside one
// segment and ** crosses segments.
func globMatch(pattern, name string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case c == '*' && i+1 < len(pattern) && pattern[i+1] == '*':
			b.WriteString(".*")
			i++
			if i+1 < len(pattern) && pattern[i+1] == '/' {
				i++
				b.WriteString("(?:")
				b.WriteString(".*/")
				b.WriteString(")?")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(name)
}

func anyGlob(patterns []string, name string) bool {
	for _, p := range patterns {
		if globMatch(p, name) {
			return true
		}
	}
	return false
}

var bumpRe = regexp.MustCompile(`(?i)\bfrom v?(\d+)(?:\.(\d+))?(?:\.(\d+))?\S* to v?(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

// bumpType reads "from 1.2.3 to 1.3.0" out of a dependency PR title.
func bumpType(title string) string {
	m := bumpRe.FindStringSubmatch(title)
	if m == nil {
		return ""
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	switch {
	case n(m[1]) != n(m[4]):
		return "major"
	case n(m[2]) != n(m[5]):
		return "minor"
	default:
		return "patch"
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// globalGates apply to every PR before any rule is considered.
func globalGates(p Policy, pr PR) []Gate {
	var denied []string
	for _, f := range pr.Files {
		if anyGlob(p.DenyPaths, f) {
			denied = append(denied, f)
		}
	}
	var blocked []string
	for _, l := range pr.Labels {
		if contains(p.BlockLabels, l) {
			blocked = append(blocked, l)
		}
	}
	return []Gate{
		{"not a draft", !pr.Draft, ""},
		{"from this repository, not a fork", !pr.Fork, ""},
		{"touches no protected path", len(denied) == 0, strings.Join(denied, ", ")},
		{"has no blocking label", len(blocked) == 0, strings.Join(blocked, ", ")},
	}
}

func ruleGates(r Rule, pr PR) []Gate {
	var gates []Gate
	if len(r.Authors) > 0 {
		gates = append(gates, Gate{"author is " + strings.Join(r.Authors, " or "), contains(r.Authors, pr.Author), pr.Author})
	}
	if r.TitlePrefix != "" {
		gates = append(gates, Gate{"title starts with " + strconv.Quote(r.TitlePrefix), strings.HasPrefix(pr.Title, r.TitlePrefix), ""})
	}
	if len(r.PathsOnly) > 0 {
		var outside []string
		for _, f := range pr.Files {
			if !anyGlob(r.PathsOnly, f) {
				outside = append(outside, f)
			}
		}
		gates = append(gates, Gate{"only touches " + strings.Join(r.PathsOnly, ", "), len(outside) == 0 && len(pr.Files) > 0, firstN(outside, 3)})
	}
	if r.MaxFiles > 0 {
		gates = append(gates, Gate{fmt.Sprintf("at most %d files", r.MaxFiles), len(pr.Files) <= r.MaxFiles, fmt.Sprintf("%d files", len(pr.Files))})
	}
	if r.MaxChangedLines > 0 {
		gates = append(gates, Gate{fmt.Sprintf("at most %d changed lines", r.MaxChangedLines), pr.ChangedLines <= r.MaxChangedLines, fmt.Sprintf("%d lines", pr.ChangedLines)})
	}
	if len(r.UpdateTypes) > 0 {
		t := bumpType(pr.Title)
		gates = append(gates, Gate{"update type is " + strings.Join(r.UpdateTypes, " or "), t != "" && contains(r.UpdateTypes, t), t})
	}
	return gates
}

func allPass(gates []Gate) bool {
	for _, g := range gates {
		if !g.Pass {
			return false
		}
	}
	return true
}

func firstN(list []string, n int) string {
	if len(list) > n {
		return strings.Join(list[:n], ", ") + fmt.Sprintf(" and %d more", len(list)-n)
	}
	return strings.Join(list, ", ")
}

// Evaluate returns the verdict for a PR: the first rule whose gates all
// pass, after the global gates. With no match the verdict carries the gates
// of the closest rule so the comment says what is missing.
func Evaluate(p Policy, pr PR) Verdict {
	global := globalGates(p, pr)
	if !allPass(global) {
		return Verdict{Gates: global, Actions: []string{ActionComment}, Reason: "A protected condition applies, so a person decides."}
	}
	var closest Verdict
	best := -1
	for _, r := range p.Rules {
		gates := ruleGates(r, pr)
		if len(gates) == 0 {
			continue
		}
		if allPass(gates) {
			return Verdict{Rule: r.Name, Gates: append(global, gates...), Actions: r.Actions, Reason: "All gates of rule " + strconv.Quote(r.Name) + " pass."}
		}
		passed := 0
		for _, g := range gates {
			if g.Pass {
				passed++
			}
		}
		if passed > best {
			best = passed
			closest = Verdict{Rule: r.Name, Gates: append(append([]Gate{}, global...), gates...)}
		}
	}
	if best < 0 {
		return Verdict{Gates: global, Actions: []string{ActionComment}, Reason: "No rule applies to this PR, so a person decides."}
	}
	closest.Actions = []string{ActionComment}
	closest.Reason = "No rule matched. Closest was " + strconv.Quote(closest.Rule) + "."
	closest.Rule = ""
	return closest
}
