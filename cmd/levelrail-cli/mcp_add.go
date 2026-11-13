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
  %[1]s mcp add <client> [--mode agent-core|read-only|standard|full] [--dry-run] [--yes]

Adds the MCP server to a client's config file. Clients: %[2]s.
Shows a diff and asks before writing; --yes skips the question (needed when
not on a terminal). The API token is referenced by environment variable and
never written into the file.
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
	c, ok := mcpClients[name]
	if !ok || fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "unknown or missing client %q, choose one of: %s\n", name, mcpClientNames())
		return exitUsage
	}
	if !agentinit.ValidMode(*mode) {
		_, _ = fmt.Fprintf(stderr, "--mode must be agent-core, read-only, standard or full, got %q\n", *mode)
		return exitUsage
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
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	var next string
	if c.toml {
		next, err = mcpEntryTOML(string(oldBytes), o)
	} else {
		next, err = mcpEntryJSON(c, string(oldBytes), o)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	diff := agentinit.Diff(string(oldBytes), next)
	if diff == "" {
		_, _ = fmt.Fprintf(stdout, "%s already up to date\n", path)
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "%s\n%s", path, diff)
	if *dryRun {
		return exitOK
	}
	if !*yes {
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
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil { //nolint:gosec // fixed per-client config location
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	_, _ = fmt.Fprintf(stderr, "Wrote %s. Set %s in the environment your client starts from.\n", path, agentinit.TokenEnvVar)
	return exitOK
}
