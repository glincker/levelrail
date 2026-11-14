package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/agentinit"
	"golang.org/x/term"
)

// mcpClient describes where one agent client keeps its MCP config and how it spells env references.
type mcpClient struct {
	name    string
	path    func(home, cwd string) string
	key     string // top-level JSON key holding servers
	envRef  string // format of an env reference, %s is the variable
	stdio   bool   // entry needs "type": "stdio"
	toml    bool
	project bool
}

var mcpClients = map[string]mcpClient{
	"claude-code": {name: "claude-code", path: func(_, cwd string) string { return filepath.Join(cwd, ".mcp.json") }, key: "mcpServers", envRef: "${%s}", project: true},
	"cursor":      {name: "cursor", path: func(_, cwd string) string { return filepath.Join(cwd, ".cursor", "mcp.json") }, key: "mcpServers", envRef: "${env:%s}", project: true},
	"vscode":      {name: "vscode", path: func(_, cwd string) string { return filepath.Join(cwd, ".vscode", "mcp.json") }, key: "servers", envRef: "${env:%s}", stdio: true, project: true},
	"windsurf": {name: "windsurf", path: func(home, _ string) string {
		return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	}, key: "mcpServers", envRef: "${env:%s}"},
	"codex": {name: "codex", path: func(home, _ string) string { return filepath.Join(home, ".codex", "config.toml") }, toml: true},
}

func mcpClientNames() string {
	var n []string
	for k := range mcpClients {
		n = append(n, k)
	}
	sort.Strings(n)
	return strings.Join(n, ", ")
}

func mcpUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s mcp add <client> [--mode agent-core|read-only|standard|full] [--dry-run] [--yes] [--json]

Adds the MCP server to a client's config file. Clients: %[2]s.
Shows a diff and asks before writing; --yes skips the question (needed when
not on a terminal). The API token is referenced by environment variable and
never written into the file.
--json prints {client, config_path, changed, dry_run, diff_summary} and never prompts.
`, prog, mcpClientNames())
}

// mcpEntryJSON merges the server into existing JSON config text and returns the new text.
func mcpEntryJSON(c mcpClient, existing string, o agentinit.Options) (string, error) {
	doc := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		if err := json.Unmarshal([]byte(existing), &doc); err != nil {
			return "", fmt.Errorf("existing config is not plain JSON (comments?), edit it by hand: %w", err)
		}
	}
	servers, _ := doc[c.key].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	env := map[string]string{agentinit.TokenEnvVar: fmt.Sprintf(c.envRef, agentinit.TokenEnvVar)}
	if o.APIURL != "" {
		env[agentinit.URLEnvVar] = o.APIURL
	}
	entry := map[string]any{"command": o.MCPBinary, "args": agentinit.MCPArgs(o.Mode), "env": env}
	if c.stdio {
		entry["type"] = "stdio"
	}
	servers[o.ServerKey] = entry
	doc[c.key] = servers
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}

// mcpEntryTOML appends a Codex server table; the token is forwarded by name only.
func mcpEntryTOML(existing string, o agentinit.Options) (string, error) {
	header := fmt.Sprintf("[mcp_servers.%s]", o.ServerKey)
	if strings.Contains(existing, header) {
		return "", fmt.Errorf("%s already exists in the Codex config, edit it by hand", header)
	}
	args := agentinit.MCPArgs(o.Mode)
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = fmt.Sprintf("%q", a)
	}
	block := fmt.Sprintf("%s\ncommand = %q\nargs = [%s]\nenv_vars = [%q]\n", header, o.MCPBinary, strings.Join(q, ", "), agentinit.TokenEnvVar)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if existing != "" {
		existing += "\n"
	}
	return existing + block, nil
}

func runMCP(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "add" {
		_, _ = fmt.Fprint(stderr, mcpUsage(prog))
		return exitUsage
	}
	fs := flag.NewFlagSet(prog+" mcp add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, mcpUsage(prog)) }
	mode := fs.String("mode", agentinit.ModeAgentCore, "MCP tool exposure: agent-core, read-only, standard or full")
	dryRun := fs.Bool("dry-run", false, "show the diff and change nothing")
	yes := fs.Bool("yes", false, "write without asking")
	jsonOut := fs.Bool("json", false, "print the result as JSON; never prompts, needs --yes to write")
	rest := args[1:]
	var name string
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		name, rest = rest[0], rest[1:]
	}
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	fail := func(msg string) int {
		if *jsonOut {
			_ = writeJSONValue(stdout, jsonError{Error: msg, Code: errCodeValidation, ExitCode: exitUsage})
			return exitUsage
		}
		_, _ = fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	c, ok := mcpClients[name]
	if !ok || fs.NArg() > 0 {
		return fail(fmt.Sprintf("unknown or missing client %q, choose one of: %s", name, mcpClientNames()))
	}
	if !agentinit.ValidMode(*mode) {
		return fail(fmt.Sprintf("--mode must be agent-core, read-only, standard or full, got %q", *mode))
	}
	home, _ := os.UserHomeDir()
	if h, ok := lookupEnv("HOME"); ok && h != "" {
		home = h
	}
	cwd, _ := os.Getwd()
	path := c.path(home, cwd)
	o := agentinit.Options{Names: initNames(prog, "", lookupEnv), Mode: *mode}
	if u, ok := lookupEnv(agentinit.URLEnvVar); ok {
		o.APIURL = u
	}
	oldBytes, err := os.ReadFile(path) //nolint:gosec // path is a fixed per-client config location
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err.Error())
	}
	var next string
	if c.toml {
		next, err = mcpEntryTOML(string(oldBytes), o)
	} else {
		next, err = mcpEntryJSON(c, string(oldBytes), o)
	}
	if err != nil {
		return fail(err.Error())
	}
	diff := agentinit.Diff(string(oldBytes), next)
	res := mcpAddResult{Client: c.name, ConfigPath: path, Changed: diff != "", DryRun: *dryRun, DiffSummary: diffSummary(diff)}
	if diff == "" {
		if *jsonOut {
			return mcpWriteJSON(stdout, stderr, res)
		}
		_, _ = fmt.Fprintf(stdout, "%s already up to date\n", path)
		return exitOK
	}
	if !*jsonOut {
		_, _ = fmt.Fprintf(stdout, "%s\n%s", path, diff)
	}
	if *dryRun {
		if *jsonOut {
			return mcpWriteJSON(stdout, stderr, res)
		}
		return exitOK
	}
	if !*yes {
		if *jsonOut {
			return fail("--json does not prompt: pass --yes to write, or --dry-run to preview")
		}
		f, isFile := stdinSource.(*os.File)
		if !isFile || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
			_, _ = fmt.Fprintln(stderr, "not a terminal: pass --yes to write, or --dry-run to preview")
			return exitUsage
		}
		_, _ = fmt.Fprint(stderr, "Write this change? [y/N] ")
		line, _ := bufio.NewReader(stdinSource).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			_, _ = fmt.Fprintln(stderr, "cancelled")
			return exitUsage
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fail(err.Error())
	}
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil { //nolint:gosec // fixed per-client config location
		return fail(err.Error())
	}
	if *jsonOut {
		return mcpWriteJSON(stdout, stderr, res)
	}
	_, _ = fmt.Fprintf(stderr, "Wrote %s. Set %s in the environment your client starts from.\n", path, agentinit.TokenEnvVar)
	return exitOK
}

// mcpAddResult is --json output; it carries counts, never the diff text or any token.
type mcpAddResult struct {
	Client      string `json:"client"`
	ConfigPath  string `json:"config_path"`
	Changed     bool   `json:"changed"`
	DryRun      bool   `json:"dry_run"`
	DiffSummary string `json:"diff_summary"`
}

func mcpWriteJSON(stdout, stderr io.Writer, v any) int {
	if err := writeJSONValue(stdout, v); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitNetwork
	}
	return exitOK
}

// diffSummary counts added and removed lines so agents can see the scale of a change.
func diffSummary(diff string) string {
	if diff == "" {
		return "no changes"
	}
	var add, del int
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++"):
			add++
		case strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---"):
			del++
		}
	}
	return fmt.Sprintf("%d lines added, %d removed", add, del)
}
