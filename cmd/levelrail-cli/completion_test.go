package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunCompletion_Dispatch(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{name: "no shell", args: nil, wantExit: exitUsage, wantStderr: "Usage:"},
		{name: "help", args: []string{"-h"}, wantExit: exitOK, wantStdout: "Usage:"},
		{name: "bash", args: []string{"bash"}, wantExit: exitOK, wantStdout: "complete -F"},
		{name: "zsh", args: []string{"zsh"}, wantExit: exitOK, wantStdout: "#compdef"},
		{name: "fish", args: []string{"fish"}, wantExit: exitOK, wantStdout: "complete -c"},
		{name: "unknown shell", args: []string{"powershell"}, wantExit: exitUsage, wantStderr: "unknown completion shell"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := runCompletion("levelrail-cli-test", tt.args, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Errorf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// TestBashCompletionScript_ValidSyntax runs the generated bash script
// through "bash -n" (parse-only, no execution) so a syntax mistake in the
// generator is caught by "go test" instead of by a user's shell.
func TestBashCompletionScript_ValidSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found on PATH")
	}

	script := bashCompletionScript("levelrail-cli")
	cmd := exec.Command(bashPath, "-n") //nolint:gosec // bashPath comes from exec.LookPath("bash"), not user input
	cmd.Stdin = strings.NewReader(script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("bash -n rejected generated script: %v\n%s\n---\n%s", err, stderr.String(), script)
	}
}

func TestFishCompletionScript_ValidSyntax(t *testing.T) {
	fishPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not found on PATH")
	}

	script := fishCompletionScript("levelrail-cli")
	cmd := exec.Command(fishPath, "--no-execute", "-c", script) //nolint:gosec // fishPath comes from exec.LookPath("fish"), not user input
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("fish --no-execute rejected generated script: %v\n%s\n---\n%s", err, stderr.String(), script)
	}
}

func TestZshCompletionScript_ValidSyntax(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not found on PATH")
	}

	script := zshCompletionScript("levelrail-cli")
	cmd := exec.Command(zshPath, "-n") //nolint:gosec // zshPath comes from exec.LookPath("zsh"), not user input
	cmd.Stdin = strings.NewReader(script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("zsh -n rejected generated script: %v\n%s\n---\n%s", err, stderr.String(), script)
	}
}

func TestCompletionScripts_CoverKnownVerbs(t *testing.T) {
	for _, gen := range []struct {
		name   string
		render func(string) string
	}{
		{"bash", bashCompletionScript},
		{"zsh", zshCompletionScript},
		{"fish", fishCompletionScript},
	} {
		script := gen.render("levelrail-cli")
		for _, want := range []string{"apps", "log-drain", "organizations", "webhook-deliveries", "migrate", "coolify"} {
			if !strings.Contains(script, want) {
				t.Errorf("%s script missing verb %q", gen.name, want)
			}
		}
	}
}

func TestSanitizeIdent(t *testing.T) {
	tests := map[string]string{
		"levelrail-cli": "levelrail_cli",
		"lr":            "lr",
		"":              "_",
		"9lr":           "_9lr",
	}
	for in, want := range tests {
		if got := sanitizeIdent(in); got != want {
			t.Errorf("sanitizeIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCommandTree_MatchesDispatchSwitches parses every non-test .go file
// in this package for "switch args[0] { case \"x\": ... }" dispatch
// blocks (the shape every run* function in this CLI uses, confirmed by
// this being the sole shape found) and compares the flattened set of
// case strings against cliCommandTree. cliCommandTree is a hand-written
// second source of truth for completion.go's generators; this test is
// the guardrail that keeps it honest as the dispatch tree grows, failing
// loudly instead of letting completions silently go stale.
func TestCommandTree_MatchesDispatchSwitches(t *testing.T) {
	dispatched := extractDispatchedVerbs(t)

	tree := map[string]bool{}
	for _, e := range walkCommandTree() {
		for _, c := range e.children {
			tree[c] = true
		}
	}

	for name := range dispatched {
		if !tree[name] {
			t.Errorf("command %q is dispatched in source (switch args[0]) but missing from cliCommandTree in completion.go", name)
		}
	}
	for name := range tree {
		if !dispatched[name] {
			t.Errorf("command %q is listed in cliCommandTree but no dispatch switch in source has a matching case anymore", name)
		}
	}
}

func extractDispatchedVerbs(t *testing.T) map[string]bool {
	t.Helper()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob source files: %v", err)
	}

	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || !isArgsZero(sw.Tag) {
				return true
			}
			collectCaseStrings(sw, found)
			return true
		})
	}
	return found
}

func isArgsZero(expr ast.Expr) bool {
	idx, ok := expr.(*ast.IndexExpr)
	if !ok {
		return false
	}
	ident, ok := idx.X.(*ast.Ident)
	if !ok || ident.Name != "args" {
		return false
	}
	lit, ok := idx.Index.(*ast.BasicLit)
	return ok && lit.Value == "0"
}

func collectCaseStrings(sw *ast.SwitchStmt, found map[string]bool) {
	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, expr := range cc.List {
			lit, ok := expr.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			if v == "-h" || v == "--help" || v == "help" {
				continue
			}
			found[v] = true
		}
	}
}
