package deploy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultStaticBuildImage is the toolchain image a static build runs in.
const DefaultStaticBuildImage = "node:22-bookworm-slim"

// EnvStaticBuildImage overrides DefaultStaticBuildImage.
const EnvStaticBuildImage = "APP_STATIC_BUILD_IMAGE"

const (
	stepStaticBuild   = "Build static site"
	stepStaticExtract = "Collect build output"
	staticStagingDir  = ".staging"
)

// StaticExtractor copies a directory out of a built image and discards the image.
type StaticExtractor interface {
	ExtractDir(ctx context.Context, tag, srcPath, destDir string) error
}

// WithStaticExtractor enables build.command for static sites.
func WithStaticExtractor(e StaticExtractor) Option {
	return func(p *Pipeline) { p.staticExtractor = e }
}

// WithStaticBuildImage sets the toolchain image static builds run in.
func WithStaticBuildImage(image string) Option {
	return func(p *Pipeline) { p.staticBuildImage = image }
}

var imageTagUnsafe = regexp.MustCompile(`[^a-z0-9_.-]+`)

// deployStaticBuilt runs build.command in a build container (so the build
// limiter applies), extracts build.publish, and publishes the tree atomically:
// the live site is only repointed once the whole output is in place.
func (p *Pipeline) deployStaticBuilt(ctx context.Context, req Request, progress func(build.ProgressEvent)) (string, error) {
	b := req.Service.Build
	fail := func(format string, a ...any) (string, error) {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, fmt.Errorf(format, a...))
	}
	if p.staticSites == nil || p.staticRootDir == "" {
		return fail("build.command needs a static site store and root directory configured")
	}
	if p.staticExtractor == nil {
		return fail("build.command needs a static build extractor configured (WithStaticExtractor)")
	}
	if err := spec.ValidateStaticBuild(req.ServiceName, b.Type, b.Command, b.Publish, b.Path, b.Args); err != nil {
		return "", fmt.Errorf("deploy: %w", err)
	}
	buildRoot, err := resolveBuildRoot(req.SourceDir, b.BaseDirectory)
	if err != nil {
		return fail("%w", err)
	}
	destRel, err := staticDestRel(req.ServiceName, req.CommitSHA)
	if err != nil {
		return fail("%w", err)
	}

	workDir, err := os.MkdirTemp("", "static-build-")
	if err != nil {
		return fail("create build workspace: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck // scratch dir
	dockerfile := filepath.Join(workDir, "Dockerfile")
	text, err := staticDockerfile(buildRoot, p.staticImage(), b)
	if err != nil {
		return fail("%w", err)
	}
	if err := os.WriteFile(dockerfile, []byte(text), 0o600); err != nil {
		return fail("write build file: %w", err)
	}

	tag := "levelrail-static-build:" + imageTagUnsafe.ReplaceAllString(strings.ToLower(req.ServiceName+"-"+req.CommitSHA), "-")
	if len(tag) > 100 {
		tag = tag[:100]
	}
	emitStatic(progress, stepStaticBuild, "running "+b.Command+" in a build container", false)
	start := time.Now()
	if _, err := p.builder.Build(ctx, build.Request{ContextDir: buildRoot, DockerfilePath: dockerfile, Tag: tag, BuildArgs: b.Args, Attest: false}, progress); err != nil {
		return fail("static build: %w (check build.command, and that build.publish %q is where it writes)", err, b.Publish)
	}
	emitStatic(progress, stepStaticBuild, "", true)

	staging, err := p.stageStaticOutput(ctx, tag, b.Publish, progress)
	if err != nil {
		return fail("%w", err)
	}
	destDir := filepath.Join(p.staticRootDir, destRel)
	if err := swapIntoPlace(staging, destDir); err != nil {
		_ = os.RemoveAll(staging)
		return fail("publish build output: %w", err)
	}
	if err := p.prepareStaticRouting(req, destDir, progress); err != nil {
		return fail("%w", err)
	}
	duration := time.Since(start)
	stats, err := HashStaticTree(destDir)
	if err != nil {
		return fail("%w", err)
	}
	if err := commitPoint(req); err != nil {
		return fail("%w", err)
	}
	held, err := p.saveStaticSite(ctx, req, store.StaticSite{Name: req.ServiceName, Domains: req.Service.Domains, RootDir: destDir})
	if err != nil {
		return fail("save static site: %w", err)
	}
	p.finishStaticPublish(ctx, req, progress, stats, duration, held)
	if p.metrics != nil {
		if err := p.metrics.RecordBuildDuration(ctx, req.ServiceName, duration, time.Now()); err != nil {
			p.logger.Warn("deploy: record build duration metric failed",
				slog.String("service", req.ServiceName), slog.String("error", err.Error()))
		}
	}
	return destDir, nil
}

func (p *Pipeline) staticImage() string {
	if p.staticBuildImage != "" {
		return p.staticBuildImage
	}
	return DefaultStaticBuildImage
}

// stageStaticOutput extracts the publish directory into a fresh directory on
// the same filesystem as the static root, removing it again on any failure.
func (p *Pipeline) stageStaticOutput(ctx context.Context, tag, publish string, progress func(build.ProgressEvent)) (string, error) {
	var token [8]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("stage build output: %w", err)
	}
	staging := filepath.Join(p.staticRootDir, staticStagingDir, hex.EncodeToString(token[:]))
	emitStatic(progress, stepStaticExtract, "copying "+publish+" out of the build", false)
	if err := p.staticExtractor.ExtractDir(ctx, tag, "/out", staging); err != nil {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("collect build output: %w", err)
	}
	empty := true
	_ = fs.WalkDir(os.DirFS(staging), ".", func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			empty = false
			return fs.SkipAll
		}
		return nil
	})
	if empty {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("build.publish %q has no files after the build, check the publish directory", publish)
	}
	emitStatic(progress, stepStaticExtract, "", true)
	return staging, nil
}

// swapIntoPlace moves the finished staging tree to dest in one rename. Any
// older tree at dest belongs to the same commit, so replacing it is safe.
func swapIntoPlace(staging, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.Rename(staging, dest); err != nil {
		return err
	}
	return nil
}

// staticDockerfile renders the throwaway build: dependencies first for layer
// caching, then the source and the user's command, then a scratch stage that
// holds only the publish directory.
func staticDockerfile(buildRoot, image string, b spec.Build) (string, error) {
	publish, err := spec.NormalizeRepoPath("build.publish", b.Publish)
	if err != nil || publish == "" {
		return "", errors.New("build.publish must be a subdirectory")
	}
	var sb strings.Builder
	sb.WriteString("FROM " + image + " AS build\nWORKDIR /src\n")
	if manifests, install := nodeInstall(buildRoot); len(manifests) > 0 {
		quoted, _ := json.Marshal(append(manifests, "./"))
		sb.WriteString("COPY " + string(quoted) + "\nRUN " + install + "\n")
	}
	sb.WriteString("COPY . .\n")
	keys := make([]string, 0, len(b.Args))
	for k := range b.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString("ARG " + k + "\nENV " + k + "=$" + k + "\n")
	}
	sb.WriteString("RUN " + b.Command + "\n")
	copyArgs, _ := json.Marshal([]string{path.Join("/src", publish), "/out/"})
	sb.WriteString("FROM scratch\nCOPY --from=build " + string(copyArgs) + "\n")
	return sb.String(), nil
}

// nodeInstall picks the dependency files to copy early and the install command
// that matches the lockfile, or nothing when the project has no package.json.
func nodeInstall(buildRoot string) (manifests []string, install string) {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(buildRoot, name))
		return err == nil
	}
	if !has("package.json") {
		return nil, ""
	}
	manifests = []string{"package.json"}
	for _, f := range []string{"package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", ".npmrc"} {
		if has(f) {
			manifests = append(manifests, f)
		}
	}
	switch {
	case has("pnpm-lock.yaml"):
		return manifests, "corepack enable && pnpm install --frozen-lockfile"
	case has("yarn.lock"):
		return manifests, "corepack enable && yarn install --frozen-lockfile"
	case has("package-lock.json"), has("npm-shrinkwrap.json"):
		return manifests, "npm ci"
	default:
		return manifests, "npm install"
	}
}
