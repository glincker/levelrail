package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/clitree"
)

const ciExamplesDir = "../../docs/ci-examples"

var cliInvocation = regexp.MustCompile(`levelrail-cli\s+([^\n|;&]+)`)

// ciExampleCommands returns every levelrail-cli invocation in the YAML's string values, with line continuations joined.
func ciExampleCommands(t *testing.T, path string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // test reads the repo's own example files
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not valid YAML: %v", path, err)
	}
	var out [][]string
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode {
			text := strings.ReplaceAll(n.Value, "\\\n", " ")
			for _, m := range cliInvocation.FindAllStringSubmatch(text, -1) {
				out = append(out, strings.Fields(strings.TrimRight(m[1], ")")))
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&doc)
	return out
}

// commandPath splits an invocation into the clitree verb path and the remaining args.
func commandPath(args []string) ([]string, []string, bool) {
	nodes := clitree.Tree
	var path []string
	for i, a := range args {
		node, ok := nodes[a]
		if !ok {
			return path, args[i:], len(path) > 0
		}
		path = append(path, a)
		if node == nil {
			return path, args[i+1:], true
		}
		nodes = node.Subs
	}
	return path, nil, len(path) > 0
}

func TestCIExamples_CommandsAndFlagsExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(ciExamplesDir, "*.yml"))
	if err != nil || len(files) < 3 {
		t.Fatalf("want at least 3 CI examples in %s, got %d (%v)", ciExamplesDir, len(files), err)
	}
	guide, err := os.ReadFile("../../docs/use-any-ci.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		name := filepath.Base(f)
		if !strings.Contains(string(guide), "ci-examples/"+name) {
			t.Errorf("docs/use-any-ci.md does not link %s", name)
		}
		var sawDeploy, sawWait bool
		for _, inv := range ciExampleCommands(t, f) {
			path, rest, ok := commandPath(inv)
			if !ok {
				t.Errorf("%s: %q is not a CLI command", name, strings.Join(inv, " "))
				continue
			}
			joined := strings.Join(path, " ")
			sawDeploy = sawDeploy || joined == "apps deploy"
			sawWait = sawWait || joined == "apps wait"
			var out, errOut bytes.Buffer
			_ = run("levelrail-cli-test", append(append([]string{}, path...), "--help"), &out, &errOut, envMap())
			help := out.String() + errOut.String()
			for _, a := range rest {
				if !strings.HasPrefix(a, "--") {
					continue
				}
				flagName := strings.SplitN(strings.TrimPrefix(a, "--"), "=", 2)[0]
				if !strings.Contains(help, "-"+flagName) {
					t.Errorf("%s: %q has no flag --%s", name, joined, flagName)
				}
			}
		}
		if !sawDeploy || !sawWait {
			t.Errorf("%s: want both apps deploy and apps wait, deploy=%v wait=%v", name, sawDeploy, sawWait)
		}
	}
}

func TestCIExamples_CommandPathRejectsUnknownVerb(t *testing.T) {
	path, rest, ok := commandPath([]string{"apps", "deploy", "web", "--image", "x"})
	if !ok || strings.Join(path, " ") != "apps deploy" || len(rest) != 3 {
		t.Fatalf("path=%v rest=%v ok=%v", path, rest, ok)
	}
	if _, _, ok := commandPath([]string{"no-such-verb"}); ok {
		t.Fatal("unknown verb accepted")
	}
}
