package deploy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/configtranslate"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// EnvProcfilePort seeds the port given to a Procfile web service, which the
// source has no way to declare.
const EnvProcfilePort = "LEVELRAIL_PROCFILE_PORT"

// maxProcfileBytes bounds a Procfile or app.json read from an untrusted checkout.
const maxProcfileBytes = 1 << 20

// ProcfileServices translates a checkout's Procfile and app.json into a
// services map when the checkout has no app spec. ok is false when the
// fallback does not apply, so existing behavior is untouched. The findings
// list every unsupported or lossy key, so the caller can show them.
func ProcfileServices(sourceDir string) (services map[string]spec.Service, findings []configtranslate.Finding, ok bool, err error) {
	if sourceDir == "" {
		return nil, nil, false, nil
	}
	if _, derr := spec.DiscoverPath(sourceDir, ""); derr == nil {
		return nil, nil, false, nil
	}
	procfile, found, err := readRepoFile(sourceDir, "Procfile")
	if err != nil || !found {
		return nil, nil, false, err
	}
	files := map[string][]byte{"Procfile": procfile}
	if b, has, rerr := readRepoFile(sourceDir, "app.json"); rerr != nil {
		return nil, nil, false, rerr
	} else if has {
		files["app.json"] = b
	}
	_, dockerErr := os.Stat(filepath.Join(sourceDir, "Dockerfile"))

	res := configtranslate.Translate(files,
		configtranslate.ForDeploy(dockerErr == nil),
		configtranslate.WithWebPort(procfilePort()))
	for _, f := range res.Findings {
		if f.Severity == configtranslate.SeverityBlocker {
			return nil, res.Findings, false, fmt.Errorf("deploy: %s: %s", f.Source, f.Message)
		}
	}
	body, err := res.Fragment.YAML()
	if err != nil {
		return nil, nil, false, fmt.Errorf("deploy: render Procfile services: %w", err)
	}
	parsed, err := spec.Parse(body)
	if err != nil {
		return nil, res.Findings, false, fmt.Errorf("deploy: Procfile services are not a valid app spec: %w", err)
	}
	return parsed.Services, res.Findings, true, nil
}

// FindingLines renders findings as one log or warning line each.
func FindingLines(fs []configtranslate.Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		line := string(f.Severity) + " " + f.Code
		if f.Subject != "" {
			line += " (" + f.Subject + ")"
		}
		out = append(out, line+": "+f.Message)
	}
	return out
}

// readRepoFile reads a regular root file, refusing symlinks so a checkout
// cannot point the loader at a file outside itself.
func readRepoFile(dir, name string) ([]byte, bool, error) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("deploy: stat %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	f, err := os.Open(path) //nolint:gosec // path is a regular file in this call's own checkout
	if err != nil {
		return nil, false, fmt.Errorf("deploy: open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxProcfileBytes))
	if err != nil {
		return nil, false, fmt.Errorf("deploy: read %s: %w", name, err)
	}
	return b, true, nil
}

func procfilePort() int {
	if v := os.Getenv(EnvProcfilePort); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	return configtranslate.DefaultWebPort
}
