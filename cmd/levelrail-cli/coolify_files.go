package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
			if err := writeSecretsEnvFile(secretsPath, a.Service.EnvLiteral); err != nil {
				return fmt.Errorf("write %s: %w", secretsPath, err)
			}
			a.SecretsFile = secretsPath
		}
	}
	return nil
}

// Mode 0600 and kept out of app.yaml so a committed migration directory leaks no secrets.
func writeSecretsEnvFile(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteString("# real secret values fetched from Coolify via --include-secret-values.\n")
	buf.WriteString("# contains live credentials: do not commit, delete once used.\n")
	for _, k := range keys {
		fmt.Fprintf(&buf, "%s=%s\n", k, values[k])
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
