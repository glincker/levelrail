package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/clitree"
	"github.com/GLINCKER/levelrail/internal/iac"
)

// Validator for the agent skills in skills/. Caps (override with env):
// SKILL_MAX_MD_BYTES (default 20480 for SKILL.md), SKILL_MAX_REF_BYTES
// (default 204800 for each reference), SKILL_DESC_MIN and SKILL_DESC_MAX
// (default 40 and 1024 characters).
// Evals are checked offline here. For a model run, feed each case prompt to an
// agent with the skill loaded and apply expected_commands and must_not; the
// scripts/mcp-eval harness only scripts MCP tool calls, it takes no prompts.

var (
	mdLinkRe     = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	inlineCmdRe  = regexp.MustCompile("`(levelrail-cli [^`]+)`")
	secretPairRe = regexp.MustCompile(`\b([A-Z][A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|KEY))=(\S+)`)
	tokenPrefix  = regexp.MustCompile(`\b(ghp_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|eyJ[A-Za-z0-9_-]{20,})`)
	useWhenRe    = regexp.MustCompile(`(?i)\b(use when|use for|use to|when (asked|a|an|the))\b`)
	// Assembled from fragments so the source itself carries no such names.
	bannedNames = []string{"cool" + "ify", "dok" + "ploy", "cap" + "rover", "ver" + "cel", "hero" + "ku", "ren" + "der.com", "rail" + "way"}
)

func skillEnvInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

func skillDirs(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	if len(dirs) == 0 {
		t.Fatal("no skill directories")
	}
	return dirs
}

func walkFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// resolveCommand checks a "levelrail-cli a b c" line against the tree. strict
// walks every word until a placeholder, flag or leaf; otherwise only two.
func resolveCommand(line string, strict bool) error {
	words := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "$ "))
	if len(words) < 2 || words[0] != "levelrail-cli" {
		return nil
	}
	nodes := clitree.Tree
	depth := 0
	for _, w := range words[1:] {
		if strings.HasPrefix(w, "<") || strings.HasPrefix(w, "-") || strings.HasPrefix(w, "[") || nodes == nil {
			return nil
		}
		n, ok := nodes[w]
		if !ok {
			return fmt.Errorf("unknown command %q in %q", w, line)
		}
		depth++
		if n == nil || (!strict && depth >= 2) {
			return nil
		}
		nodes = n.Subs
	}
	return nil
}

func parseFrontMatter(text string) (map[string]string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, false
	}
	fm := map[string]string{}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return fm, true
		}
		if k, v, ok := strings.Cut(lines[i], ":"); ok {
			fm[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return nil, false
}

// lintText applies the content rules every skill file must pass.
func lintText(t *testing.T, path, text string) {
	t.Helper()
	// Generated references mirror importer command names; evals list bad strings on purpose.
	generated := strings.Contains(filepath.ToSlash(path), "/references/commands/")
	isEval := filepath.Base(path) == "evals.json"
	for i, line := range strings.Split(text, "\n") {
		at := fmt.Sprintf("%s:%d", path, i+1)
		if strings.ContainsRune(line, rune(0x2013)) || strings.ContainsRune(line, rune(0x2014)) {
			t.Errorf("%s: em or en dash", at)
		}
		low := strings.ToLower(line)
		for _, b := range bannedNames {
			if !generated && strings.Contains(low, b) {
				t.Errorf("%s: competitor name %q", at, b)
			}
		}
		if isEval {
			continue
		}
		if tokenPrefix.MatchString(line) {
			t.Errorf("%s: token-like string", at)
		}
		for _, m := range secretPairRe.FindAllStringSubmatch(line, -1) {
			v := strings.Trim(m[2], "`\"'.,;)")
			if v == "" || strings.ContainsAny(v[:1], "<$") {
				continue
			}
			if iac.LooksSecret(m[1], v) {
				t.Errorf("%s: hard-coded secret value for %s", at, m[1])
			}
		}
	}
}

func commandLines(text string) map[int]string {
	out := map[int]string{}
	inFence := false
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		s := strings.TrimPrefix(strings.TrimSpace(line), "$ ")
		if inFence && strings.HasPrefix(s, "levelrail-cli ") {
			out[i+1] = s
		}
		for _, m := range inlineCmdRe.FindAllStringSubmatch(line, -1) {
			out[i+1] = m[1]
		}
	}
	return out
}

func TestSkillsValidate(t *testing.T) {
	maxMD := skillEnvInt("SKILL_MAX_MD_BYTES", 20480)
	maxRef := skillEnvInt("SKILL_MAX_REF_BYTES", 204800)
	descMin := skillEnvInt("SKILL_DESC_MIN", 40)
	descMax := skillEnvInt("SKILL_DESC_MAX", 1024)
	for _, dir := range skillDirs(t) {
		name := filepath.Base(dir)
		skillPath := filepath.Join(dir, "SKILL.md")
		raw, err := os.ReadFile(skillPath) //nolint:gosec // fixed repo path
		if err != nil {
			t.Errorf("%s: %v", skillPath, err)
			continue
		}
		if len(raw) > maxMD {
			t.Errorf("%s: %d bytes exceeds cap %d", skillPath, len(raw), maxMD)
		}
		text := string(raw)
		fm, ok := parseFrontMatter(text)
		if !ok {
			t.Errorf("%s:1: missing front matter", skillPath)
			continue
		}
		inFM := true
		for i, line := range strings.Split(text, "\n") {
			if i > 0 && strings.TrimSpace(line) == "---" {
				inFM = false
			}
			if inFM && strings.Contains(line, "\t") {
				t.Errorf("%s:%d: tab in front matter", skillPath, i+1)
			}
		}
		if fm["name"] != name {
			t.Errorf("%s: name %q must equal directory %q", skillPath, fm["name"], name)
		}
		desc := fm["description"]
		if n := len(desc); n < descMin || n > descMax {
			t.Errorf("%s: description is %d characters, want %d to %d", skillPath, n, descMin, descMax)
		}
		if !useWhenRe.MatchString(desc) {
			t.Errorf("%s: description must say when to use the skill", skillPath)
		}
		for _, f := range walkFiles(t, dir) {
			data, err := os.ReadFile(f) //nolint:gosec // walked repo path
			if err != nil {
				t.Fatal(err)
			}
			lintText(t, f, string(data))
			if !strings.HasSuffix(f, ".md") {
				continue
			}
			if f != skillPath && len(data) > maxRef {
				t.Errorf("%s: %d bytes exceeds cap %d", f, len(data), maxRef)
			}
			for i, line := range strings.Split(string(data), "\n") {
				for _, m := range mdLinkRe.FindAllStringSubmatch(line, -1) {
					target := m[1]
					if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
						continue
					}
					target, _, _ = strings.Cut(target, "#")
					if _, err := os.Stat(filepath.Join(filepath.Dir(f), target)); err != nil { //nolint:gosec // repo-local skill files
						t.Errorf("%s:%d: dead link %s", f, i+1, m[1])
					}
				}
			}
			if f == skillPath {
				for ln, cmd := range commandLines(string(data)) {
					if err := resolveCommand(cmd, false); err != nil {
						t.Errorf("%s:%d: %v", f, ln, err)
					}
				}
			}
		}
	}
}

type skillEvalCase struct {
	ID               string   `json:"id"`
	Prompt           string   `json:"prompt"`
	ExpectedCommands []string `json:"expected_commands"`
	MustInclude      []string `json:"must_include"`
	MustNot          []string `json:"must_not"`
}

type skillEvalFile struct {
	Skill string          `json:"skill"`
	Cases []skillEvalCase `json:"cases"`
}

// Offline: checks eval shape and that expected commands exist, so evals cannot rot.
func TestSkillEvalsShape(t *testing.T) {
	covered := 0
	for _, dir := range skillDirs(t) {
		name := filepath.Base(dir)
		path := filepath.Join(dir, "evals", "evals.json")
		raw, err := os.ReadFile(path) //nolint:gosec // fixed repo path
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		var f skillEvalFile
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if f.Skill != name {
			t.Errorf("%s: skill %q must equal %q", path, f.Skill, name)
		}
		if n := len(f.Cases); n < 5 || n > 10 {
			t.Errorf("%s: %d cases, want 5 to 10", path, n)
		}
		seen := map[string]bool{}
		for _, c := range f.Cases {
			covered++
			if c.ID == "" || seen[c.ID] {
				t.Errorf("%s: empty or duplicate id %q", path, c.ID)
			}
			seen[c.ID] = true
			if strings.TrimSpace(c.Prompt) == "" || len(c.ExpectedCommands) == 0 || len(c.MustNot) == 0 {
				t.Errorf("%s: case %s needs prompt, expected_commands and must_not", path, c.ID)
			}
			for _, p := range c.ExpectedCommands {
				if !strings.HasPrefix(p, "levelrail-cli ") {
					t.Errorf("%s: case %s prefix %q must start with levelrail-cli", path, c.ID, p)
				} else if err := resolveCommand(p, true); err != nil {
					t.Errorf("%s: case %s: %v", path, c.ID, err)
				}
			}
		}
	}
	if covered == 0 {
		t.Fatal("no eval cases found")
	}
}
