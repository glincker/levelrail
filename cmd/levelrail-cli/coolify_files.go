package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A generation failure demotes only that app to blocking, not the whole run.
func writeMigrationFiles(outDir string, report *migrationReport) error {
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create out-dir %s: %w", outDir, err)
	}

	for i := range report.Apps {
		a := &report.Apps[i]
		if a.Blocking {
			continue
		}

		data, err := buildAppYAML(*a)
		if err != nil {
			a.Blocking = true
			a.Issues = append(a.Issues, migrationIssue{Field: "yaml", Severity: issueBlocking, Detail: err.Error()})
			continue
		}

		path := filepath.Join(outDir, a.ServiceName+".yaml")
		if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // out-dir is operator-supplied CLI input, an app.yaml is not secret material
			return fmt.Errorf("write %s: %w", path, err)
		}

		if len(a.Service.EnvLiteral) > 0 {
			secretsPath := filepath.Join(outDir, a.ServiceName+".secrets.env")
			skipped, err := writeSecretsEnvFile(secretsPath, a.Service.EnvLiteral)
			if err != nil {
				return fmt.Errorf("write %s: %w", secretsPath, err)
			}
			a.Issues = envSkippedIssues(a.Issues, skipped)
			a.SecretsFile = secretsPath
		}
		if err := writeAppFiles(outDir, a); err != nil {
			a.Issues = append(a.Issues, migrationIssue{Field: "file_storages", Severity: issueReview, Detail: err.Error()})
		}
	}
	return nil
}

// Mode 0600 and kept out of app.yaml so a committed migration directory leaks no secrets.
// A value with a line break or NUL would forge extra lines, so it is skipped and named.
func writeSecretsEnvFile(path string, values map[string]string) ([]string, error) {
	keys := make([]string, 0, len(values))
	var skipped []string
	for k, v := range values {
		if strings.ContainsAny(k+v, "\r\n\x00") {
			skipped = append(skipped, k)
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(skipped)
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteString("# real secret values fetched from Coolify via --include-secret-values.\n")
	buf.WriteString("# contains live credentials: do not commit, delete once used.\n")
	for _, k := range keys {
		fmt.Fprintf(&buf, "%s=%s\n", k, values[k])
	}
	return skipped, os.WriteFile(path, buf.Bytes(), 0o600)
}

func envSkippedIssues(issues []migrationIssue, skipped []string) []migrationIssue {
	for _, k := range skipped {
		issues = append(issues, migrationIssue{Field: k, Severity: issueReview, Detail: "value spans lines and was not written to the env file, set it with apps secrets set"})
	}
	return issues
}

// writeAppFiles stages copied files under <name>.files at their mount path, owner-only since one may be secret.
// Paths were validated as clean absolute file paths, so the join cannot leave the directory.
func writeAppFiles(outDir string, a *mappedApp) error {
	base := filepath.Join(outDir, a.ServiceName+".files")
	var errs []string
	for i := range a.Files {
		f := &a.Files[i]
		dst := filepath.Join(base, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			errs = append(errs, "file "+f.Path+" was not staged: "+err.Error())
			continue
		}
		if err := os.WriteFile(dst, f.Content, 0o600); err != nil {
			errs = append(errs, "file "+f.Path+" was not staged: "+err.Error())
			continue
		}
		f.StagedPath = dst
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
