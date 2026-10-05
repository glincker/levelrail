package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/agentinit"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/kit/stackdetect"
)

// File actions reported by init.
const (
	initCreate    = "create"
	initOverwrite = "overwrite"
	initUnchanged = "unchanged"
	initSkip      = "skip"
)

type initFile struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
	Diff   string `json:"diff,omitempty"`

	content string
}

type initStack struct {
	Provider string   `json:"provider"`
	Label    string   `json:"label"`
	Build    string   `json:"build"`
	Port     int      `json:"port,omitempty"`
	Notes    []string `json:"notes,omitempty"`
}

type initResult struct {
	Dir     string     `json:"dir"`
	Stack   initStack  `json:"stack"`
	Mode    string     `json:"mode"`
	APIURL  string     `json:"api_url,omitempty"`
	Files   []initFile `json:"files"`
	DryRun  bool       `json:"dry_run"`
	Written bool       `json:"written"`
}

// runInit implements "init": detect the stack in a project directory and
// write app.yaml, AGENTS.md and .mcp.json.
func runInit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "init", "print the plan and result as JSON to stdout and nothing else", stderr)
	var dir, mode, mcpBinary string
	var force, dryRun, yes bool
	fs.StringVar(&dir, "dir", ".", "project directory to initialise")
	fs.StringVar(&mode, "mode", agentinit.ModeAgentCore, "MCP tool exposure written to .mcp.json: agent-core (small tool profile, default), read-only, standard or full")
	fs.StringVar(&mcpBinary, "mcp-binary", "", "MCP server binary name (default: derived from the CLI name)")
	fs.BoolVar(&force, "force", false, "overwrite files that already exist (a diff is shown first)")
	fs.BoolVar(&dryRun, "dry-run", false, "show what would be written and change nothing")
	fs.BoolVar(&yes, "yes", false, "write without asking for confirmation")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, initUsage(prog)) }

	_, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if !agentinit.ValidMode(mode) {
		return reportError(stdout, stderr, jsonOut, newValidationError("--mode must be agent-core, read-only, standard or full, got %q", mode))
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	if apiURL == defaultAPIURL && apiURLFlag == "" {
		apiURL = ""
	}

	stack, err := stackdetect.Detect(dir)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("detect stack in %s: %w", dir, err))
	}
	opts := agentinit.Options{Names: initNames(prog, mcpBinary, lookupEnv), APIURL: apiURL, Mode: mode, App: stack.Name}

	files, err := planInit(dir, stack, opts, force)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	res := initResult{
		Dir:    dir,
		Stack:  initStack{Provider: stack.Provider, Label: stack.Label, Build: stack.Build, Port: stack.Port, Notes: stack.Notes},
		Mode:   mode,
		APIURL: apiURL,
		Files:  files,
		DryRun: dryRun,
	}

	pending := 0
	for _, f := range files {
		if f.Action == initCreate || f.Action == initOverwrite {
			pending++
		}
	}
	if !dryRun && pending > 0 {
		if !jsonOut && of.Format != outputJSON {
			printInitPlan(stderr, res)
		}
		if !yes {
			if err := confirmInit(stderr, pending); err != nil {
				return reportError(stdout, stderr, jsonOut, newValidationError("%s", err))
			}
		}
		if err := writeInit(dir, files); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		res.Written = true
	}

	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if dryRun || pending == 0 {
			printInitPlan(stdout, res)
		}
		printInitSummary(stdout, prog, res)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func initNames(prog, mcpBinary string, lookupEnv func(string) (string, bool)) agentinit.Names {
	stem := strings.TrimSuffix(filepath.Base(prog), "-cli")
	pick := func(env, fallback string) string {
		if v, ok := lookupEnv(env); ok && v != "" {
			return v
		}
		return fallback
	}
	bin := pick("APP_BRAND_BINARY_NAME", stem)
	if mcpBinary == "" {
		mcpBinary = bin + "-mcp"
	}
	return agentinit.Names{
		Product:   pick("APP_BRAND_NAME", stem),
		CLI:       prog,
		MCPBinary: mcpBinary,
		ServerKey: strings.ToLower(pick("APP_BRAND_SHORT_NAME", stem)),
	}
}

// planInit decides what to do with each generated file without touching disk.
func planInit(dir string, stack stackdetect.Stack, opts agentinit.Options, force bool) ([]initFile, error) {
	specPath := "app.yaml"
	if found, err := spec.DiscoverPath(dir, ""); err == nil {
		specPath = filepath.Base(found)
	}

	var files []initFile
	if stack.Detected() {
		body, err := agentinit.AppYAML(stack)
		if err != nil {
			return nil, err
		}
		files = append(files, initFile{Path: specPath, content: body})
	} else {
		files = append(files, initFile{Path: specPath, Action: initSkip, Reason: "no stack detected: write app.yaml by hand"})
	}
	agentsMD, err := agentinit.AgentsMD(opts)
	if err != nil {
		return nil, err
	}
	mcp, err := agentinit.MCPJSON(opts)
	if err != nil {
		return nil, err
	}
	files = append(files, initFile{Path: "AGENTS.md", content: agentsMD}, initFile{Path: ".mcp.json", content: mcp})

	for i := range files {
		f := &files[i]
		if f.Action != "" {
			continue
		}
		existing, err := os.ReadFile(filepath.Join(dir, f.Path)) //nolint:gosec // path is under the operator-chosen project dir
		switch {
		case errors.Is(err, os.ErrNotExist):
			f.Action = initCreate
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", f.Path, err)
		case string(existing) == f.content:
			f.Action = initUnchanged
		default:
			f.Diff = agentinit.Diff(string(existing), f.content)
			if force {
				f.Action = initOverwrite
			} else {
				f.Action, f.Reason = initSkip, "already exists, pass --force to overwrite"
			}
		}
	}
	return files, nil
}

func writeInit(dir string, files []initFile) error {
	for _, f := range files {
		if f.Action != initCreate && f.Action != initOverwrite {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, f.Path), []byte(f.content), 0o644); err != nil { //nolint:gosec // project files meant to be committed
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
	}
	return nil
}

func confirmInit(stderr io.Writer, n int) error {
	f, ok := stdinSource.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to write without confirmation, or --dry-run to preview")
	}
	_, _ = fmt.Fprintf(stderr, "Write %d file(s)? [y/N] ", n)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

func printInitPlan(w io.Writer, res initResult) {
	label := res.Stack.Label
	if label == "" {
		label = "none detected"
	}
	_, _ = fmt.Fprintf(w, "Stack: %s\n", label)
	for _, n := range res.Stack.Notes {
		_, _ = fmt.Fprintf(w, "  note: %s\n", n)
	}
	for _, f := range res.Files {
		line := fmt.Sprintf("  %-9s %s", f.Action, f.Path)
		if f.Reason != "" {
			line += " (" + f.Reason + ")"
		}
		_, _ = fmt.Fprintln(w, line)
		for _, d := range strings.Split(strings.TrimSuffix(f.Diff, "\n"), "\n") {
			if d != "" {
				_, _ = fmt.Fprintf(w, "      %s\n", d)
			}
		}
	}
}

func printInitSummary(w io.Writer, prog string, res initResult) {
	switch {
	case res.DryRun:
		_, _ = fmt.Fprintln(w, "Dry run: nothing was written.")
	case res.Written:
		_, _ = fmt.Fprintf(w, "Done. Set %s in your environment, then run \"%s apps wait <name>\" after a deploy.\n", agentinit.TokenEnvVar, prog)
	default:
		_, _ = fmt.Fprintln(w, "Nothing to write.")
	}
}

func initUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s init [--dir DIR] [--mode MODE] [--dry-run] [--force] [--yes] [flags]

Detects the project's stack (Dockerfile, compose file, package.json, go.mod,
Python, Java, static site) and writes three files into the directory:

  app.yaml    the app spec, validated with the platform's own validator
  AGENTS.md   how an AI agent deploys, checks and rolls back this app
  .mcp.json   MCP server config; the token is an env var reference, never a value

Existing files are never overwritten unless --force is given; a diff of what
would change is shown either way. Without --yes, a terminal is asked to confirm;
a script must pass --yes or --dry-run.

--mode selects the tool exposure written to .mcp.json:
  agent-core   small tool profile for deploy and debug work (default)
  read-only    every read tool, no mutations
  standard     read and mutating tools, no destructive ones
  full         every tool

--api-url (or the active profile) is written into AGENTS.md and .mcp.json when known.
--mcp-binary overrides the MCP server binary name.
`, prog)
}
