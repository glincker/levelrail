package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/kit/stackdetect"
)

type appDetectResult struct {
	Path         string   `json:"path"`
	Detected     bool     `json:"detected"`
	Language     string   `json:"language"`
	Framework    string   `json:"framework"`
	Build        string   `json:"build"`
	Port         int      `json:"port"`
	Name         string   `json:"name"`
	DeployCmd    string   `json:"deploy_command"`
	Notes        []string `json:"notes"`
	BuildCommand string   `json:"build_command,omitempty"`
	Publish      string   `json:"publish,omitempty"`
}

func appsDetectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps detect [path] [--json]

Inspects a project directory offline (no network, nothing is executed) and
prints the detected language, framework, build method, port and a suggested
command to create app.yaml. Defaults to the current directory.
`, prog)
}

// languageOf maps a Railpack provider id to a language; providers with no
// language of their own (static, compose, dockerfile) report themselves.
func languageOf(provider string) string {
	switch provider {
	case "node", "deno", "bun":
		return "javascript"
	case "staticfile", "static":
		return "static"
	case "":
		return ""
	}
	return provider
}

func buildAppDetect(prog, dir string) (appDetectResult, error) {
	st, err := stackdetect.Detect(dir)
	if err != nil {
		return appDetectResult{}, err
	}
	res := appDetectResult{Path: dir, Detected: st.Detected(), Language: languageOf(st.Provider), Framework: st.Label,
		Build: st.Build, Port: st.Port, Name: st.Name, Notes: st.Notes, BuildCommand: st.BuildCommand, Publish: st.Publish}
	if res.Notes == nil {
		res.Notes = []string{}
	}
	if st.Detected() {
		cmd := []string{prog, "apps", "init", "--build", st.Build}
		if st.Name != "" {
			cmd = append(cmd, "--name", st.Name)
		}
		if st.Port > 0 {
			cmd = append(cmd, "--port", fmt.Sprint(st.Port))
		}
		res.DeployCmd = strings.Join(cmd, " ")
	}
	return res, nil
}

func runAppsDetect(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps detect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDetectUsage(prog)) }
	jsonOut := fs.Bool("json", false, "print the result as JSON to stdout and nothing else")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() > 1 {
		_, _ = fmt.Fprint(stderr, appsDetectUsage(prog))
		return exitUsage
	}
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	res, err := buildAppDetect(prog, dir)
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, fmt.Errorf("detect stack in %s: %w", dir, err))
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return exitOK
	}
	if !res.Detected {
		_, _ = fmt.Fprintln(stdout, "no buildable stack detected")
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "language:  %s\nframework: %s\nbuild:     %s\nport:      %d\ndeploy:    %s\n", res.Language, res.Framework, res.Build, res.Port, res.DeployCmd)
	for _, n := range res.Notes {
		_, _ = fmt.Fprintf(stdout, "note:      %s\n", n)
	}
	return exitOK
}
