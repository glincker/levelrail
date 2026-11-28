package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/clitree"
)

const userAliasesFile = "aliases.json"

func userAliasesPath(prog string) (string, error) {
	dir, err := configDir(prog)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, userAliasesFile), nil
}

func loadUserAliases(prog string) map[string]string {
	path, err := userAliasesPath(prog)
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is the CLI's own config file
	if err != nil {
		return nil
	}
	m := map[string]string{}
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

func saveUserAliases(prog string, m map[string]string) error {
	path, err := userAliasesPath(prog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// expandUserAlias replaces args[0] once; built-in verbs always win so an alias cannot hide a command.
func expandUserAlias(prog string, args []string) []string {
	if len(args) == 0 || args[0] == "alias" || args[0] == "__complete" {
		return args
	}
	if _, builtin := clitree.Tree[args[0]]; builtin {
		return args
	}
	exp, ok := loadUserAliases(prog)[args[0]]
	if !ok {
		return args
	}
	words := strings.Fields(exp)
	if len(words) == 0 {
		return args
	}
	return append(words, args[1:]...)
}

func validAliasName(name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsAny(name, " \t/\\") {
		return fmt.Errorf("alias name %q must be a single word without a leading dash", name)
	}
	if _, builtin := clitree.Tree[name]; builtin {
		return fmt.Errorf("alias name %q is a built-in command", name)
	}
	return nil
}

// runAlias implements "alias set|list|delete".
func runAlias(prog string, args []string, stdout, stderr io.Writer) int {
	usage := fmt.Sprintf("Usage:\n  %[1]s alias set <name> <command...>\n  %[1]s alias list\n  %[1]s alias delete <name>\n", prog)
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, usage)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	m := loadUserAliases(prog)
	if m == nil {
		m = map[string]string{}
	}
	switch args[0] {
	case "list":
		names := make([]string, 0, len(m))
		for n := range m {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			_, _ = fmt.Fprintf(stdout, "%s = %s\n", n, m[n])
		}
		return exitOK
	case "set":
		if len(args) < 3 {
			_, _ = fmt.Fprint(stderr, usage)
			return exitUsage
		}
		if err := validAliasName(args[1]); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		m[args[1]] = strings.Join(args[2:], " ")
	case "delete":
		if len(args) != 2 {
			_, _ = fmt.Fprint(stderr, usage)
			return exitUsage
		}
		if _, ok := m[args[1]]; !ok {
			_, _ = fmt.Fprintf(stderr, "no alias named %q\n", args[1])
			return exitUsage
		}
		delete(m, args[1])
	default:
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	if err := saveUserAliases(prog, m); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitValidation
	}
	return exitOK
}
