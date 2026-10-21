package deploy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

const stepStaticRoutes = "Compile routing rules"

// WithStaticRouteLimits sets the caps on a static site's routing rules.
func WithStaticRouteLimits(l spec.StaticLimits) Option {
	return func(p *Pipeline) { p.staticRouteLimits = l }
}

func (p *Pipeline) routeLimits() spec.StaticLimits {
	if p.staticRouteLimits.MaxRules == 0 {
		return spec.DefaultStaticLimits()
	}
	return p.staticRouteLimits
}

// prepareStaticRouting compiles the _redirects and _headers files in the
// published tree plus the app.yaml routing fields, stores the result beside
// the tree and removes the two files so they are never served. It runs before
// the commit point, so a failure leaves the previous deploy's rules serving.
func (p *Pipeline) prepareStaticRouting(req Request, destDir string, progress func(build.ProgressEvent)) error {
	l := p.routeLimits()
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("open published files: %w", err)
	}
	defer root.Close() //nolint:errcheck // handle only used for confined reads
	in := req.Service.StaticRoutingInput()
	for _, f := range []struct {
		name string
		dst  *string
	}{{spec.StaticSourceRedirects, &in.Redirects}, {spec.StaticSourceHeaders, &in.Headers}} {
		text, err := readRulesFile(root, f.name, l.MaxFileBytes)
		if err != nil {
			return err
		}
		*f.dst = text
	}
	rs, err := spec.CompileStatic(in, l)
	if err != nil {
		return fmt.Errorf("routing rules: %w", err)
	}
	for _, pr := range rs.Problems {
		emitStatic(progress, stepStaticRoutes, fmt.Sprintf("%s: %s:%d: %s (line skipped)", pr.Severity, pr.Source, pr.Line, pr.Message), false)
	}
	for _, name := range []string{spec.StaticSourceRedirects, spec.StaticSourceHeaders} {
		if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s from the published files: %w", name, err)
		}
	}
	if err := spec.WriteStaticSidecar(destDir, rs); err != nil {
		return err
	}
	emitStatic(progress, stepStaticRoutes, fmt.Sprintf("%d rules, %d header blocks, %d problems", len(rs.Rules), len(rs.Headers), len(rs.Problems)), true)
	return nil
}

func readRulesFile(root *os.Root, name string, limit int) (string, error) {
	f, err := root.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open %s: %w", name, err)
	}
	defer f.Close() //nolint:errcheck // read-only handle
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s must be a regular file", name)
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	return string(raw), nil
}
