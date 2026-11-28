package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/experimental"
)

type docsNode struct {
	Path     []string   `json:"path"`
	Children []string   `json:"children,omitempty"`
	Help     string     `json:"help,omitempty"`
	Flags    []docsFlag `json:"flags,omitempty"`
	Example  string     `json:"example,omitempty"`
}

// docsFlag is one flag parsed from a command's "Flags:" help block.
type docsFlag struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Default string `json:"default,omitempty"`
	Usage   string `json:"usage,omitempty"`
}

var (
	flagHeadRe    = regexp.MustCompile(`^ {1,4}--?([A-Za-z0-9][A-Za-z0-9._-]*)(?: +(\S.*?))?\s*$`)
	flagDefaultRe = regexp.MustCompile(`\(default ("[^"]*"|[^)]*)\)\s*$`)
)

// parseHelpFlags reads the stdlib flag layout ("  -name type" then a tab-indented
// usage line) and the hand-written "  --name   usage" layout from a Flags: block.
func parseHelpFlags(help string) []docsFlag {
	lines := strings.Split(help, "\n")
	var out []docsFlag
	in := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "Flags:" {
			in = true
			continue
		}
		if !in {
			continue
		}
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, " ") {
			in = false
			continue
		}
		m := flagHeadRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		f := docsFlag{Name: m[1]}
		rest := m[2]
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "    \t") {
			// Stdlib layout: rest is the type, the next line is the usage.
			f.Type = rest
			f.Usage = strings.TrimSpace(lines[i+1])
			i++
		} else if idx := strings.Index(rest, "  "); idx >= 0 {
			f.Type = strings.TrimSpace(rest[:idx])
			f.Usage = strings.TrimSpace(rest[idx:])
		} else {
			f.Usage = rest
		}
		if d := flagDefaultRe.FindStringSubmatch(f.Usage); d != nil {
			f.Default = strings.Trim(d[1], `"`)
		}
		out = append(out, f)
	}
	return out
}

// firstExample returns the first command line under an Example(s) heading.
func firstExample(help string) string {
	lines := strings.Split(help, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "Example") {
			continue
		}
		head, rest, ok := strings.Cut(t, ":")
		if !ok || (head != "Example" && head != "Examples") {
			continue
		}
		if r := strings.TrimSpace(rest); strings.HasPrefix(r, "levelrail-cli ") {
			return r
		}
		for _, next := range lines[i+1:] {
			n := strings.TrimSpace(next)
			if strings.HasPrefix(n, "levelrail-cli ") {
				return n
			}
			if n == "" && !strings.HasPrefix(next, " ") && strings.HasPrefix(next, "Flags") {
				break
			}
		}
	}
	return ""
}

func docsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s docs [--format markdown|json|man | --json] [--out-dir DIR]

Dumps the command tree with each group's help text. Offline and
deterministic, so skills and docs can be generated from it.
--json is the same as --format json.
--out-dir writes one markdown file per top-level command plus INDEX.md.
--format man needs --out-dir and writes one levelrail-cli-<command>.1 page per
top-level command. JSON also lists every leaf command with its flags (name,
type, default) and one example when the help text has one.
`, prog)
}

// helpFor runs the real dispatcher with --help so docs never drift from usage text.
func helpFor(path []string) string {
	var out, errb bytes.Buffer
	args := append(append([]string{}, path...), "--help")
	code := run("levelrail-cli", args, &out, &errb, func(string) (string, bool) { return "", false })
	text := out.String()
	if text == "" && code == exitOK {
		text = errb.String()
	}
	if text == "" {
		text = errb.String()
	}
	return strings.TrimSpace(text)
}

func collectDocs() []docsNode { return collectDocsOpt(false) }

// collectDocsOpt with leaves also documents commands that have no subcommands,
// which the machine-readable output needs for their flags and examples.
func collectDocsOpt(leaves bool) []docsNode {
	// Help text varies with enabled features; document all of them so output is host independent.
	prev := experimental.EnabledList()
	experimental.Set(experimental.All()...)
	defer experimental.Set(prev...)
	entries := walkFullCommandTree()
	groups := map[string]bool{}
	for _, e := range entries {
		groups[e.path] = true
	}
	mk := func(path, children []string) docsNode {
		help := helpFor(path)
		return docsNode{Path: path, Children: children, Help: help, Flags: parseHelpFlags(help), Example: firstExample(help)}
	}
	var nodes []docsNode
	for _, e := range entries {
		path := strings.Fields(e.path)
		if e.path != "" {
			nodes = append(nodes, mk(path, e.children))
		}
		if !leaves {
			continue
		}
		for _, c := range e.children {
			lp := append(append([]string{}, path...), c)
			if !groups[strings.Join(lp, " ")] {
				nodes = append(nodes, mk(lp, nil))
			}
		}
	}
	return nodes
}

func renderDocsMarkdown(nodes []docsNode, topOnly string) string {
	var b strings.Builder
	for _, n := range nodes {
		if topOnly != "" && n.Path[0] != topOnly {
			continue
		}
		fmt.Fprintf(&b, "## levelrail-cli %s\n\n", strings.Join(n.Path, " "))
		if len(n.Children) > 0 {
			fmt.Fprintf(&b, "Subcommands: %s\n\n", strings.Join(n.Children, ", "))
		}
		if n.Help != "" {
			fmt.Fprintf(&b, "```\n%s\n```\n\n", n.Help)
		}
	}
	return b.String()
}

// roffText escapes one help line so roff keeps it verbatim.
func roffText(line string) string {
	line = strings.ReplaceAll(line, `\`, `\e`)
	line = strings.ReplaceAll(line, "-", `\-`)
	if strings.HasPrefix(line, ".") || strings.HasPrefix(line, "'") {
		line = `\&` + line
	}
	return line
}

// docsManPages returns levelrail-cli-<top>.1 content per top-level command.
func docsManPages(nodes []docsNode) map[string]string {
	pages := map[string]*strings.Builder{}
	for _, n := range nodes {
		top := n.Path[0]
		b := pages[top]
		if b == nil {
			b = &strings.Builder{}
			pages[top] = b
			fmt.Fprintf(b, ".TH %s 1 \"\" \"levelrail-cli\"\n.SH NAME\n%s \\- Levelrail CLI %s commands\n.SH COMMANDS\n",
				strings.ToUpper(roffText("levelrail-cli-"+top)), roffText("levelrail-cli "+top), roffText(top))
		}
		fmt.Fprintf(b, ".SS %s\n", roffText("levelrail-cli "+strings.Join(n.Path, " ")))
		if len(n.Children) > 0 {
			fmt.Fprintf(b, "Subcommands: %s\n.PP\n", roffText(strings.Join(n.Children, ", ")))
		}
		if n.Help != "" {
			b.WriteString(".nf\n")
			for _, l := range strings.Split(n.Help, "\n") {
				b.WriteString(roffText(l) + "\n")
			}
			b.WriteString(".fi\n")
		}
	}
	files := map[string]string{}
	for top, b := range pages {
		files["levelrail-cli-"+top+".1"] = b.String()
	}
	return files
}

// docsFiles returns file name to content for --out-dir.
func docsFiles(nodes []docsNode) map[string]string {
	files := map[string]string{}
	tops := map[string]bool{}
	for _, n := range nodes {
		tops[n.Path[0]] = true
	}
	var names []string
	for t := range tops {
		names = append(names, t)
		files[t+".md"] = "# levelrail-cli " + t + "\n\nGenerated by `levelrail-cli docs`. Do not edit.\n\n" + renderDocsMarkdown(nodes, t)
	}
	sort.Strings(names)
	var idx strings.Builder
	idx.WriteString("# Command reference\n\nGenerated by `levelrail-cli docs`. Do not edit. Run `levelrail-cli <command> --help` for flags.\n\n")
	for _, t := range names {
		fmt.Fprintf(&idx, "- [%s](%s.md)\n", t, t)
	}
	files["INDEX.md"] = idx.String()
	return files
}

func runDocs(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" docs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, docsUsage(prog)) }
	format := fs.String("format", "markdown", "markdown, json or man")
	jsonFlag := fs.Bool("json", false, "same as --format json")
	outDir := fs.String("out-dir", "", "write markdown files to this directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if *jsonFlag {
		*format = "json"
	}
	if fs.NArg() > 0 || (*format != "markdown" && *format != "json" && *format != "man") || (*format == "man" && *outDir == "") {
		_, _ = fmt.Fprint(stderr, docsUsage(prog))
		return exitUsage
	}
	nodes := collectDocsOpt((*format == "json" && *outDir == "") || *format == "man")
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o750); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		files := docsFiles(nodes)
		if *format == "man" {
			files = docsManPages(nodes)
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(*outDir, name), []byte(body), 0o600); err != nil {
				_, _ = fmt.Fprintln(stderr, err)
				return exitUsage
			}
		}
		return exitOK
	}
	if *format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(nodes)
		return exitOK
	}
	_, _ = fmt.Fprint(stdout, "# levelrail-cli command reference\n\n"+renderDocsMarkdown(nodes, ""))
	return exitOK
}
