package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// envValueSourceMaxBytes bounds one value read from a file or stdin so a wrong path cannot flood the app spec.
const envValueSourceMaxBytes = 64 << 10

// envSetStdin is swapped in tests; KEY=@- reads from it.
var envSetStdin io.Reader = os.Stdin

// runEnvSet implements "env set <app> KEY=VALUE...": the plain-env counterpart of "env push" for a few values.
func runEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "env set", "print the plan as JSON to stdout and nothing else", stderr)
	var apply, noApply bool
	fs.BoolVar(&apply, "apply", false, "restart the app now so the new env reaches the running container")
	fs.BoolVar(&noApply, "no-apply", false, "leave the change pending even when apps.env_apply_on_set is on")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, envSetUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if envApplyConflict(stderr, prog, apply, noApply) {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: env set requires an app name and at least one KEY=VALUE\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	entries := make([]envFileEntry, 0, len(rest)-1)
	stdinUsed := false
	for _, kv := range rest[1:] {
		k, v, found := strings.Cut(kv, "=")
		if !found || k == "" {
			// Never echo the argument: it may carry a secret value.
			return reportError(stdout, stderr, jsonOut, newValidationError("%q is not KEY=VALUE", kvKeyHint(kv)))
		}
		resolved, err := resolveEnvValue(k, v, &stdinUsed)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		entries = append(entries, envFileEntry{Key: k, Value: resolved})
	}

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}
	secretKeys, err := appSecretKeys(ctx, client, name, app.SecretEnv)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if bound, berr := client.ListBoundEnv(ctx, name); berr == nil {
		for _, b := range bound {
			secretKeys = append(secretKeys, b.Key)
		}
	}
	var rebuildKeys []string
	plan := planEnvImport(app.Env, entries, secretKeys, false)
	result := envImportResult{App: name, Plan: plan}
	warnEnvSecrets(ctx, client, stderr, name, plan)
	if len(plan.Secret) > 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s is stored as a secret; change it with \"%s apps secrets set %s KEY VALUE\"", strings.Join(plan.Secret, ", "), prog, name))
	}
	if len(plan.New) > 0 || len(plan.Changed) > 0 {
		app.Env = plan.Merged
		saved, err := client.UpdateApp(ctx, name, app)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update env for app %q: %w", name, err))
		}
		rebuildKeys = saved.RebuildKeys
		result.Applied = true
	}
	code := writeScheduledTaskResult(stdout, stderr, of, result, func() { printEnvImportHuman(stdout, name, result) })
	if result.Applied {
		hintOut := stdout
		if of.Format == outputJSON || of.Query != "" {
			hintOut = io.Discard
		}
		gitApp := false
		if len(rebuildKeys) > 0 {
			_, gerr := client.GetGitSource(ctx, name)
			gitApp = gerr == nil
		}
		printRebuildHint(hintOut, prog, name, rebuildKeys, gitApp)
		doApply, reason := resolveEnvApply(ctx, client, apply, noApply)
		if applyCode := afterConfigWriteReason(ctx, client, prog, name, doApply, reason, hintOut, stderr); code == exitOK {
			code = applyCode
		}
	}
	return code
}

// kvKeyHint trims an argument to the part before any "=" so errors never print a value.
func kvKeyHint(kv string) string {
	k, _, _ := strings.Cut(kv, "=")
	return k
}

// resolveEnvValue expands "@-", "@./path", "@/abs" and "@@literal"; a bare "@word" stays literal so existing values keep working.
// Errors name the key and path, never the content.
func resolveEnvValue(key, v string, stdinUsed *bool) (string, error) {
	if !strings.HasPrefix(v, "@") {
		return v, nil
	}
	if strings.HasPrefix(v, "@@") {
		return v[1:], nil
	}
	src := v[1:]
	if src != "-" && !isExplicitPath(src) {
		return v, nil
	}
	var raw []byte
	var err error
	if src == "-" {
		if *stdinUsed {
			return "", newValidationError("only one KEY=@- is allowed per call, stdin can be read once")
		}
		*stdinUsed = true
		raw, err = readBounded(envSetStdin)
		if err != nil {
			return "", newValidationError("%s: cannot read stdin: %v", key, err)
		}
	} else {
		raw, err = readEnvValueFile(src)
		if err != nil {
			return "", newValidationError("%s: cannot read %s: %v", key, src, err)
		}
	}
	if !utf8.Valid(raw) || strings.IndexByte(string(raw), 0) >= 0 {
		return "", newValidationError("%s: value from %s must be valid UTF-8 without NUL bytes", key, src)
	}
	return strings.TrimSuffix(string(raw), "\n"), nil
}

// isExplicitPath keeps a value like "@oncall" literal: only absolute or ./ ../ forms read a file.
func isExplicitPath(p string) bool {
	return filepath.IsAbs(p) || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") ||
		strings.HasPrefix(p, ".\\") || strings.HasPrefix(p, "..\\")
}

func readEnvValueFile(path string) ([]byte, error) {
	// Stat before Open so a FIFO or device never blocks the read.
	if st, err := os.Stat(path); err != nil {
		return nil, errReadReason(err)
	} else if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	// The path is the operator's own CLI argument.
	f, err := os.Open(path) //nolint:gosec // G304: explicit user-supplied file source
	if err != nil {
		return nil, errReadReason(err)
	}
	defer func() { _ = f.Close() }()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	return readBounded(f)
}

// errReadReason drops the wrapped path so the message carries only the cause.
func errReadReason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func readBounded(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, envValueSourceMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read failed")
	}
	if len(b) > envValueSourceMaxBytes {
		return nil, fmt.Errorf("value is larger than %d bytes", envValueSourceMaxBytes)
	}
	return b, nil
}

// printRebuildHint names the command that rebuilds this app's source; the CLI cannot tell upload from image-only apps cheaply.
func printRebuildHint(w io.Writer, prog, app string, keys []string, gitApp bool) {
	for _, k := range keys {
		if gitApp {
			_, _ = fmt.Fprintf(w, "%s is baked in at build time: run %s deploy %s --latest to apply\n", k, prog, app)
			continue
		}
		_, _ = fmt.Fprintf(w, "%s is baked in at build time: run %s deploy <dir> from your project folder to apply, or rebuild the image where it is built\n", k, prog)
	}
}

func envSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s env set <app> KEY=VALUE [KEY=VALUE...] [flags]

Sets plain environment variables on an app, creating or changing them.
Only the first "=" splits, so values may contain "=", spaces and quotes (quote them for your shell).
KEY=@./path or KEY=@/abs/path reads the value from a file (one trailing newline is dropped), KEY=@- reads stdin (once per call),
and KEY=@@text sets the literal "@text". Any other leading @ (for example @oncall) is stored as typed. Sources are limited to 64 KiB of UTF-8 text.
Keys stored as secrets are refused, use "%[1]s apps secrets set" for those.
Changes reach the running container at the next restart, or now with --apply.
NEXT_PUBLIC_ and VITE_ keys are baked in at build time: a git app needs "%[1]s deploy <app> --latest", an upload app "%[1]s deploy <dir>".

Flags:
  --apply             restart the app now so the new env reaches the running container
  --no-apply          leave the change pending even when apps.env_apply_on_set is on
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the plan as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
