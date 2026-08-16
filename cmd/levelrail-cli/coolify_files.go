package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// writeMigrationFiles writes one app.yaml per non-blocking app in
// report.Apps into outDir, and a companion *.secrets.env file for any
// app whose EnvLiteral was populated (--include-secret-values). A
// generation failure (buildAppYAML's own schema validation) demotes that
// one app to blocking rather than aborting the whole run, so one bad
// mapping doesn't lose every other app's already-generated file.
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

// writeSecretsEnvFile writes real env var values fetched from Coolify to
// a plain KEY=VALUE file, mode 0600: this is live credential material,
// deliberately kept out of the generated app.yaml (see buildAppYAML's
// own doc comment) so an operator who commits their migrated app.yaml
// directory doesn't also commit its secrets by accident.
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
