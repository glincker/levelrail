package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/mcptools"
)

// configOut is where --print-config writes; a variable so tests can capture it.
var configOut io.Writer = os.Stdout

var configClients = []string{"claude", "cursor", "vscode", "zed"}

// tokenPlaceholder stands in for the API token: a real one is never printed.
const tokenPlaceholder = "<your API token>" //nolint:gosec // placeholder text, not a credential

type serverEntry struct {
	command string
	args    []string
	env     map[string]string
}

func runPrintConfig(prog string, f cliFlags, lookupEnv func(string) (string, bool)) error {
	profile := resolveProfile(f.profile, lookupEnv)
	apiURL := resolveAPIURL(f.apiURL, lookupEnv, prog, profile)
	block, err := renderConfig(f.printConfig, prog, buildServerEntry(prog, f, apiURL, lookupEnv))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(configOut, block)
	return err
}

func buildServerEntry(prog string, f cliFlags, apiURL string, lookupEnv func(string) (string, bool)) serverEntry {
	mode := f.mode
	if mode == "" {
		mode, _ = lookupEnv(mcptools.EnvMode)
	}
	if mode == "" {
		mode = string(mcptools.ModeReadOnly)
	}
	args := []string{"--mode", mode}
	if f.allowWrite {
		args = append(args, "--allow-write")
	}
	if f.team != "" {
		args = append(args, "--team", f.team)
	}
	if f.project != "" {
		args = append(args, "--project", f.project)
	}
	return serverEntry{
		command: prog,
		args:    args,
		env: map[string]string{
			apiclient.EnvAPIToken: tokenPlaceholder,
			apiclient.EnvAPIURL:   apiURL,
		},
	}
}

func renderConfig(client, name string, e serverEntry) (string, error) {
	var doc map[string]any
	switch strings.ToLower(client) {
	case "claude", "cursor":
		doc = map[string]any{"mcpServers": map[string]any{name: map[string]any{
			"command": e.command, "args": e.args, "env": e.env,
		}}}
	case "vscode":
		doc = map[string]any{"servers": map[string]any{name: map[string]any{
			"type": "stdio", "command": e.command, "args": e.args, "env": e.env,
		}}}
	case "zed":
		doc = map[string]any{"context_servers": map[string]any{name: map[string]any{
			"command": map[string]any{"path": e.command, "args": e.args, "env": e.env},
		}}}
	default:
		return "", fmt.Errorf("unknown --print-config client %q, want one of %s", client, strings.Join(configClients, ", "))
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return "", fmt.Errorf("render %s config: %w", client, err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
