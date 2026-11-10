package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"golang.org/x/term"
)

// runMigrateHub implements "migrate hub sessions <verb>", the CLI side of the
// database migration hub the migrate page drives.
func runMigrateHub(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, migrateHubUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, migrateHubUsage(prog))
		return exitOK
	case "sessions":
		return runMigrateHubSessions(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown migrate hub subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, migrateHubUsage(prog))
		return exitUsage
	}
}

func runMigrateHubSessions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, migrateHubUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, migrateHubUsage(prog))
		return exitOK
	case "list":
		return runHubSessionsList(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return runHubSessionsShow(prog, rest, stdout, stderr, lookupEnv)
	case "select":
		return runHubSessionsSelect(prog, rest, stdout, stderr, lookupEnv)
	case "step":
		return runHubSessionsStep(prog, rest, stdout, stderr, lookupEnv)
	case "apply":
		return runHubSessionsApply(prog, rest, stdout, stderr, lookupEnv)
	case "receipt":
		return runHubSessionsReceipt(prog, rest, stdout, stderr, lookupEnv)
	case "connection":
		return runHubSessionsConnection(prog, rest, false, stdout, stderr, lookupEnv)
	case "reveal":
		return runHubSessionsConnection(prog, rest, true, stdout, stderr, lookupEnv)
	case "delete":
		return runHubSessionsDelete(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown migrate hub sessions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, migrateHubUsage(prog))
		return exitUsage
	}
}

func migrateHubUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate hub sessions list [flags]                     sessions, without their databases
  %[1]s migrate hub sessions show <id> [flags]                one session's databases, checks and progress
  %[1]s migrate hub sessions select <id> [--select DB] [--deselect DB] [--target-name DB=NAME] [--target-version DB=VER]
  %[1]s migrate hub sessions step <id> <inventory|preflight|verify|cutover> [flags]
  %[1]s migrate hub sessions apply <id> [--password-stdin] [--yes] [flags]  start copying the selected databases
  %[1]s migrate hub sessions receipt <id> [--out FILE [--force]] [flags]  the read-only receipt, secrets masked
  %[1]s migrate hub sessions connection <id> <db> [flags]       a verified database's connection, no secret
  %[1]s migrate hub sessions reveal <id> <db> [--yes] [flags]   the same with its password (read:sensitive, audited)
  %[1]s migrate hub sessions delete <id> [--yes] [flags]        drop a session and the password held for it

Selection and apply need write:sensitive. The source password is never a flag:
apply reuses the one held for the session, or reads one line with --password-stdin.
`, prog)
}

// hubFlagSet is apiFlagSet plus the shared usage text, so every hub verb
// documents its own flags through PrintDefaults.
func hubFlagSet(prog, verb, jsonHelp string, stderr io.Writer) (*flag.FlagSet, apiFlagPtrs) {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "migrate hub sessions "+verb, jsonHelp, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, migrateHubUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	return fs, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}
}

func runHubSessionsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := hubFlagSet(prog, "list", "print the sessions as JSON", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	if _, ok := requireArgs(fs, stderr, prog, "migrate hub sessions list", "no arguments", 0); !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListMigrationSessions(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list migration sessions: %w", err))
	}
	if list == nil {
		list = []apiclient.HubSession{}
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "No migration session.")
		}
		for _, s := range list {
			_, _ = fmt.Fprintf(stdout, "%s  %s %s:%d  step %s  running %t\n", s.ID, s.Engine, s.Host, s.Port, s.Step, s.Running)
		}
	})
}

func runHubSessionsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := hubFlagSet(prog, "show", "print the session as JSON", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate hub sessions show", "exactly one session id", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.GetMigrationSession(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("show migration session %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() { printHubSessionDetail(stdout, s) })
}

func printHubSessionDetail(w io.Writer, s apiclient.HubSession) {
	_, _ = fmt.Fprintf(w, "session %s  %s %s:%d (%s)  step %s  running %t\n", s.ID, s.Engine, s.Host, s.Port, s.ServerVersion, s.Step, s.Running)
	for _, it := range s.Items {
		mark := " "
		if it.Selected {
			mark = "x"
		}
		_, _ = fmt.Fprintf(w, "  [%s] %s -> %s (%s)  %s", mark, it.SourceDB, it.TargetName, it.TargetVersion, it.Status)
		if it.Reason != "" {
			_, _ = fmt.Fprintf(w, ": %s", it.Reason)
		}
		_, _ = fmt.Fprintln(w)
	}
	sm := s.Summary
	_, _ = fmt.Fprintf(w, "selected %d/%d, blocked %d, verified %d, failed %d, can apply %t\n", sm.Selected, sm.Databases, sm.Blocked, sm.Verified, sm.Failed, sm.CanApply)
}

func runHubSessionsSelect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := hubFlagSet(prog, "select", "print the updated session as JSON", stderr)
	var sel, desel, names, versions listFlag
	fs.Var(&sel, "select", "a source database to copy, repeatable")
	fs.Var(&desel, "deselect", "a source database to leave out, repeatable")
	fs.Var(&names, "target-name", "DB=NAME: the target database name for DB, repeatable")
	fs.Var(&versions, "target-version", "DB=VERSION: the target engine version for DB, repeatable")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate hub sessions select", "exactly one session id", 1)
	if !ok {
		return exitUsage
	}
	items, err := hubSelections(sel, desel, names, versions)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
	}
	if len(items) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("nothing to change: pass --select, --deselect, --target-name or --target-version"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.SelectMigrationItems(context.Background(), rest[0], items)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update selection of %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() { printHubSessionDetail(stdout, s) })
}

// hubSelections folds the flags into one entry per source database, in the
// order each database was first named.
func hubSelections(sel, desel, names, versions []string) ([]apiclient.HubSelection, error) {
	byDB := map[string]*apiclient.HubSelection{}
	var order []string
	get := func(db string) *apiclient.HubSelection {
		if e, ok := byDB[db]; ok {
			return e
		}
		e := &apiclient.HubSelection{SourceDB: db}
		byDB[db] = e
		order = append(order, db)
		return e
	}
	for _, list := range []struct {
		dbs []string
		on  bool
	}{{sel, true}, {desel, false}} {
		for _, db := range list.dbs {
			v := list.on
			get(db).Selected = &v
		}
	}
	for _, kv := range []struct {
		pairs []string
		flag  string
	}{{names, "--target-name"}, {versions, "--target-version"}} {
		for _, p := range kv.pairs {
			db, val, ok := strings.Cut(p, "=")
			if !ok || db == "" || val == "" {
				return nil, fmt.Errorf("%s %q must look like DB=VALUE", kv.flag, p)
			}
			v := val
			if kv.flag == "--target-name" {
				get(db).TargetName = &v
			} else {
				get(db).TargetVersion = &v
			}
		}
	}
	out := make([]apiclient.HubSelection, 0, len(order))
	for _, db := range order {
		out = append(out, *byDB[db])
	}
	return out, nil
}

func runHubSessionsStep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := hubFlagSet(prog, "step", "print the updated session as JSON", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate hub sessions step", "a session id and a step", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.SetMigrationSessionStep(context.Background(), rest[0], rest[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("move session %q to %s: %w", rest[0], rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() { printHubSessionDetail(stdout, s) })
}

func runHubSessionsApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := hubFlagSet(prog, "apply", "print the session as JSON", stderr)
	passwordStdin := fs.Bool("password-stdin", false, "read the source password from the first line of piped stdin instead of the one held for the session")
	yes := fs.Bool("yes", false, "start without asking; required when stdin is not a terminal")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate hub sessions apply", "exactly one session id", 1)
	if !ok {
		return exitUsage
	}
	password := ""
	if *passwordStdin {
		p, err := readPipedPassword(*yes)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		password = p
	}
	if !*yes {
		if err := confirmDestructive(stderr, fmt.Sprintf("Copy the selected databases of session %s now?", rest[0])); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.ApplyMigrationSession(context.Background(), rest[0], password)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply migration session %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() { printHubSessionDetail(stdout, s) })
}

// readPipedPassword reads one line from a pipe; a terminal is refused so the
// password is never echoed, and --yes is needed since stdin cannot also confirm.
func readPipedPassword(yes bool) (string, error) {
	if !yes {
		return "", newValidationError("--password-stdin needs --yes, since stdin then holds the password and cannot answer a prompt")
	}
	if f, ok := stdinSource.(*os.File); ok && term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return "", newValidationError("--password-stdin reads a pipe, not a terminal: pipe the password in, for example from your password manager")
	}
	line, err := bufio.NewReader(stdinSource).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	p := strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(p) == "" {
		return "", newValidationError("--password-stdin read an empty password")
	}
	return p, nil
}

func runHubSessionsReceipt(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := hubFlagSet(prog, "receipt", "print the receipt as JSON", stderr)
	out := fs.String("out", "", "write the receipt JSON to FILE (mode 0600) instead of stdout; refuses an existing file")
	force := fs.Bool("force", false, "with --out, replace an existing regular file")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate hub sessions receipt", "exactly one session id", 1)
	if !ok {
		return exitUsage
	}
	if *out != "" && (*ptrs.output != "" || *ptrs.query != "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("--out writes the whole receipt as JSON; drop --query and --output"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	raw, err := client.MigrationSessionReceipt(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("receipt of session %q: %w", rest[0], err))
	}
	if *out == "" {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("decode receipt: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, v, func() { _ = writeJSONValue(stdout, v) })
	}
	body, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := writePrivateFile(*out, append(body, '\n'), *force); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	_, _ = fmt.Fprintf(stderr, "receipt written to %s\n", *out)
	return exitOK
}

// writePrivateFile writes through a 0600 temp file in the target's directory
// and renames it, so a symlink or other non-regular target is never written through.
func writePrivateFile(path string, body []byte, force bool) error {
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return newValidationError("%s exists and is not a regular file; choose another --out", path)
		}
		if !force {
			return newValidationError("%s already exists; pass --force to replace it", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".receipt-*.json")
	if err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}
	name := tmp.Name()
	_, werr := tmp.Write(body)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, 0o600)
	}
	if werr == nil {
		werr = os.Rename(name, path)
	}
	if werr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("write receipt: %w", werr)
	}
	return nil
}

func runHubSessionsConnection(prog string, args []string, reveal bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "connection"
	if reveal {
		verb = "reveal"
	}
	fs, ptrs := hubFlagSet(prog, verb, "print the connection as JSON", stderr)
	yes := new(bool)
	if reveal {
		yes = fs.Bool("yes", false, "print the password without asking; required when stdin is not a terminal")
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate hub sessions "+verb, "a session id and a source database", 2)
	if !ok {
		return exitUsage
	}
	if reveal && !*yes {
		if err := confirmDestructive(stderr, fmt.Sprintf("Print the password of %s to this terminal? It is audited.", rest[1])); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var conn apiclient.HubConnection
	var err error
	if reveal {
		conn, err = client.RevealMigrationConnection(context.Background(), rest[0], rest[1])
	} else {
		conn, err = client.MigrationConnection(context.Background(), rest[0], rest[1])
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s of %s in session %q: %w", verb, rest[1], rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, conn, func() {
		_, _ = fmt.Fprintf(stdout, "%s %s on %s:%d user %s\n", conn.Engine, conn.Database, conn.Host, conn.Port, conn.Username)
		for _, e := range conn.Env {
			_, _ = fmt.Fprintf(stdout, "  %s=%s\n", e.EnvVar, e.Value)
		}
		if conn.Reference != "" {
			_, _ = fmt.Fprintf(stdout, "reference: %s\n", conn.Reference)
		}
	})
}

func runHubSessionsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := hubFlagSet(prog, "delete", "print the result as JSON", stderr)
	yes := fs.Bool("yes", false, "delete without asking; required when stdin is not a terminal")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate hub sessions delete", "exactly one session id", 1)
	if !ok {
		return exitUsage
	}
	if !*yes {
		if err := confirmDestructive(stderr, fmt.Sprintf("Delete migration session %s?", rest[0])); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteMigrationSession(context.Background(), rest[0]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete migration session %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"id": rest[0], "status": "deleted"}, func() {
		_, _ = fmt.Fprintf(stdout, "deleted migration session %s\n", rest[0])
	})
}
