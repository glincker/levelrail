package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

const redactedValue = "[redacted]"

// Words that mark a key or setting name as holding a secret; matched case-insensitively as substrings.
var sensitiveKeyWords = []string{"token", "secret", "password", "passwd", "key", "credential", "cookie", "authorization", "dsn", "webhook"}

func isSensitiveName(name string) bool {
	n := strings.ToLower(name)
	for _, w := range sensitiveKeyWords {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// redactTree masks secret-looking keys, the value and default of a sensitive runtime setting, and any literal copy of the caller's own token.
func redactTree(v any, token string) any {
	switch t := v.(type) {
	case map[string]any:
		settingKey, _ := t["key"].(string)
		settingSecret := settingKey != "" && isSensitiveName(settingKey)
		out := make(map[string]any, len(t))
		for k, val := range t {
			if isSensitiveName(k) && k != "key" && !isEmptyValue(val) || settingSecret && (k == "value" || k == "default") && !isEmptyValue(val) {
				out[k] = redactedValue
				continue
			}
			out[k] = redactTree(val, token)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactTree(val, token)
		}
		return out
	case string:
		if token != "" {
			return strings.ReplaceAll(t, token, redactedValue)
		}
		return t
	}
	return v
}

func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case bool:
		return false
	}
	return false
}

// toTree round-trips any value through JSON so redaction sees plain maps and slices.
func toTree(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return tree, nil
}

type bundleFile struct {
	name string
	data any
}

// writeDoctorBundle writes a redacted zip for a bug report. A section that fails is noted in errors.txt so a partial bundle still helps.
func writeDoctorBundle(w io.Writer, files []bundleFile, failures []string, token string, now time.Time) error {
	zw := zip.NewWriter(w)
	add := func(name string, body []byte) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return err
		}
		_, err = f.Write(body)
		return err
	}
	for _, bf := range files {
		tree, err := toTree(bf.data)
		if err != nil {
			failures = append(failures, bf.name+": "+err.Error())
			continue
		}
		body, err := json.MarshalIndent(redactTree(tree, token), "", "  ")
		if err != nil {
			failures = append(failures, bf.name+": "+err.Error())
			continue
		}
		if err := add(bf.name, append(body, '\n')); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		if err := add("errors.txt", []byte(strings.Join(failures, "\n")+"\n")); err != nil {
			return err
		}
	}
	return zw.Close()
}

// runDoctorBundle gathers versions, doctor checks, runtime settings and node state, then saves them redacted to path.
func runDoctorBundle(ctx context.Context, path, prog, tokenFlag, apiURLFlag, profileFlag string, local bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), jsonOut bool) int {
	now := time.Now().UTC()
	if path == "" {
		path = "levelrail-doctor-bundle-" + now.Format("20060102-150405") + ".zip"
	}
	meta := map[string]any{"generated_at": now.Format(time.RFC3339), "cli": localVersionInfo(), "os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version()}
	files := []bundleFile{}
	var failures []string
	token := ""
	if local || !doctorServerConfigured(tokenFlag, apiURLFlag, profileFlag, prog, lookupEnv) {
		meta["mode"] = "local only, no control plane configured"
		files = append(files, bundleFile{"doctor.json", localDoctorReport(ctx, lookupEnv)})
	} else {
		meta["mode"] = "control plane"
		token = resolveToken(tokenFlag, lookupEnv, prog, resolveProfile(profileFlag, lookupEnv))
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		if up, err := client.GetUpdates(ctx); err != nil {
			failures = append(failures, "server version: "+err.Error())
		} else {
			meta["server"] = up
		}
		if rep, err := client.GetSystemDoctor(ctx); err != nil {
			failures = append(failures, "doctor: "+err.Error())
		} else {
			files = append(files, bundleFile{"doctor.json", rep})
		}
		if st, err := client.GetRuntimeSettings(ctx); err != nil {
			failures = append(failures, "settings: "+err.Error())
		} else {
			files = append(files, bundleFile{"settings.json", st})
		}
		if nodes, err := client.ListNodes(ctx); err != nil {
			failures = append(failures, "nodes: "+err.Error())
		} else {
			for i := range nodes {
				nodes[i].Address, nodes[i].CertFingerprint = "", ""
			}
			files = append(files, bundleFile{"nodes.json", nodes})
		}
	}
	files = append([]bundleFile{{"versions.json", meta}}, files...)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the path is the one the operator asked for
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create bundle: %w", err))
	}
	if err := writeDoctorBundle(f, files, failures, token, now); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write bundle: %w", err))
	}
	if err := f.Close(); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write bundle: %w", err))
	}
	_, _ = fmt.Fprintf(stdout, "support bundle written to %s (secrets redacted; skim it before attaching it to an issue)\n", path)
	if len(failures) > 0 {
		_, _ = fmt.Fprintf(stderr, "%d section(s) could not be collected, see errors.txt in the bundle\n", len(failures))
	}
	return exitOK
}
