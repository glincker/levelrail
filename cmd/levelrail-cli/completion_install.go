package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runCompletionInstall writes the completion script for the detected shell and says what to add.
// It never edits a shell rc file; fish autoloads from its completions directory so needs no line.
func runCompletionInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" completion install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	shell := fs.String("shell", "", "bash, zsh or fish (default: detected from $SHELL)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	sh := *shell
	if sh == "" {
		v, _ := lookupEnv("SHELL")
		sh = filepath.Base(v)
	}
	var script, rc string
	switch sh {
	case "bash":
		script, rc = bashCompletionScript(prog), "~/.bashrc"
	case "zsh":
		script, rc = zshCompletionScript(prog), "~/.zshrc"
	case "fish":
		script = fishCompletionScript(prog)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: cannot tell the shell from %q, pass --shell bash|zsh|fish\n", prog, sh)
		return exitUsage
	}
	dir, err := completionInstallDir(prog, sh, lookupEnv)
	if err != nil {
		return reportError(stdout, stderr, false, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return reportError(stdout, stderr, false, err)
	}
	target := filepath.Join(dir, prog+"."+sh)
	if err := os.WriteFile(target, []byte(script), 0o644); err != nil { //nolint:gosec // a completion script holds no secret and must be readable by the shell
		return reportError(stdout, stderr, false, err)
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", target)
	if rc != "" {
		_, _ = fmt.Fprintf(stdout, "add this line to %s:\n  source %s\n", rc, strings.Replace(target, homeOrEmpty(lookupEnv), "~", 1))
	}
	_, _ = fmt.Fprintln(stdout, "then restart your shell")
	return exitOK
}

func homeOrEmpty(lookupEnv func(string) (string, bool)) string {
	h, _ := lookupEnv("HOME")
	if h == "" {
		return "\x00"
	}
	return h
}

func completionInstallDir(prog, shell string, lookupEnv func(string) (string, bool)) (string, error) {
	if shell == "fish" {
		if x, _ := lookupEnv("XDG_CONFIG_HOME"); x != "" {
			return filepath.Join(x, "fish", "completions"), nil
		}
		h, _ := lookupEnv("HOME")
		if h == "" {
			var err error
			if h, err = os.UserHomeDir(); err != nil {
				return "", err
			}
		}
		return filepath.Join(h, ".config", "fish", "completions"), nil
	}
	base, err := configDir(prog)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "completions"), nil
}
