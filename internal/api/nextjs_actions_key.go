package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// nextActionsKeyEnv is the variable Next.js reads for the Server Actions key.
const nextActionsKeyEnv = "NEXT_SERVER_ACTIONS_ENCRYPTION_KEY"

// nextActionsMinMajor is the first Next.js version with encrypted action closures.
const nextActionsMinMajor = 14

const nextDetectFileCap = 256 << 10

// ensureNextActionsKey stores a random Server Actions key once for an app the
// wizard detected as Next.js and marks it a runtime-only secret on req. It
// never touches a build: secrets are not passed to builds.
func (rt *Router) ensureNextActionsKey(ctx context.Context, req *deploy.Request, name, framework string) {
	if !strings.EqualFold(strings.TrimSpace(framework), "Next.js") || rt.secrets == nil {
		return
	}
	// A nil settings service is a unit-test router: the default (on) applies.
	if rt.runtimeSettings != nil && !rt.runtimeSettings.Bool(ctx, runtimesettings.KeyNextjsGenerateActionsKey) {
		return
	}
	if _, set := req.Service.Env[nextActionsKeyEnv]; set {
		return
	}
	if !nextMajorAtLeast(req.SourceDir, req.Service.Build.BaseDirectory, nextActionsMinMajor) {
		return
	}
	unlock := lockAppName(name)
	defer unlock()
	exists, err := rt.secrets.Exists(ctx, name, nextActionsKeyEnv)
	if err != nil {
		rt.logger.Warn("api: next.js actions key check failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	if !exists {
		raw := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			rt.logger.Warn("api: next.js actions key generation failed", slog.String("name", name))
			return
		}
		if err := rt.secrets.SetValueGuarded(ctx, name, nextActionsKeyEnv, base64.StdEncoding.EncodeToString(raw), false); err != nil {
			rt.logger.Warn("api: next.js actions key store failed", slog.String("name", name), slog.String("error", err.Error()))
			return
		}
	}
	if req.Service.Env == nil {
		req.Service.Env = map[string]spec.EnvVar{}
	}
	req.Service.Env[nextActionsKeyEnv] = spec.EnvVar{Secret: true}
}

// nextMajorAtLeast reads the app directory's top level and asks the wizard
// detector for the Next.js major version.
func nextMajorAtLeast(sourceDir, baseDir string, least int) bool {
	dir := filepath.Join(sourceDir, filepath.Clean("/"+baseDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Name())
	}
	read := func(p string) (string, bool) {
		f, err := os.Open(filepath.Join(dir, filepath.Base(p))) //nolint:gosec // names come from ReadDir of the build checkout
		if err != nil {
			return "", false
		}
		defer func() { _ = f.Close() }()
		b, err := io.ReadAll(io.LimitReader(f, nextDetectFileCap))
		return string(b), err == nil
	}
	d := appwizard.Detect(appwizard.Input{Paths: paths, Read: read})
	return d.Next != nil && d.Next.Major >= least
}
