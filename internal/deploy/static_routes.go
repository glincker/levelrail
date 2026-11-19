package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StaticSiteSettingsReader is optionally implemented by the static site store
// so a publish carries the rules and options edited in the dashboard.
type StaticSiteSettingsReader interface {
	GetStaticSiteSettings(ctx context.Context, name string) (store.StaticSiteSettings, store.StaticUIRules, error)
}

const stepStaticRoutes = "Compile routing rules"

// ErrProxyTargetDenied is what a ProxyAuthorizer returns for an app the
// acting principal may not use, and for one that does not exist, so the
// deploy message is no existence oracle.
var ErrProxyTargetDenied = errors.New("not found or not accessible")

// ProxyAuthorizer checks one proxy target and returns its scope key.
type ProxyAuthorizer func(ctx context.Context, app string) (scope string, err error)

// authorizeProxyRules refuses a deploy whose proxy rules name an app the
// actor cannot use. Previews drop proxy rules instead, since a pull request
// is untrusted input.
func authorizeProxyRules(ctx context.Context, req Request, rs *spec.StaticRuleset, progress func(build.ProgressEvent)) error {
	if req.Preview {
		if n := rs.ProxyRuleCount(); n > 0 {
			rs.Rules = slices.DeleteFunc(rs.Rules, func(r spec.StaticRule) bool { return r.IsProxy() })
			emitStatic(progress, stepStaticRoutes, fmt.Sprintf("warning: %d proxy rules are not served on previews", n), false)
		}
		return nil
	}
	for _, pr := range rs.ProxyRules() {
		app, _ := pr.ProxyTarget()
		scope, err := "", ErrProxyTargetDenied
		if req.ProxyAuthorizer != nil {
			scope, err = req.ProxyAuthorizer(ctx, app)
		}
		if errors.Is(err, ErrProxyTargetDenied) {
			return fmt.Errorf("routing rules: %s: proxy target app %q was not found or you lack read access to it", pr.Source, app)
		}
		if err != nil {
			return fmt.Errorf("routing rules: %s: check proxy target: %w", pr.Source, err)
		}
		if scope != "" {
			if rs.ProxyScopes == nil {
				rs.ProxyScopes = map[string]string{}
			}
			rs.ProxyScopes[app] = scope
		}
	}
	return nil
}

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
func (p *Pipeline) prepareStaticRouting(ctx context.Context, req Request, destDir string, progress func(build.ProgressEvent)) error {
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
	if err := p.applyStaticUI(ctx, req.ServiceName, rs, l); err != nil {
		return fmt.Errorf("routing rules: %w", err)
	}
	for _, pr := range rs.ProxyRules() {
		if app, _ := pr.ProxyTarget(); app == req.ServiceName {
			return fmt.Errorf("routing rules: %s: a site cannot proxy to itself", pr.Source)
		}
	}
	if err := authorizeProxyRules(ctx, req, rs, progress); err != nil {
		return err
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

// applyStaticUI folds the dashboard rules and option overrides into rs, so a
// rollback to this tree restores them with the sidecar.
func (p *Pipeline) applyStaticUI(ctx context.Context, name string, rs *spec.StaticRuleset, l spec.StaticLimits) error {
	reader, ok := p.staticSites.(StaticSiteSettingsReader)
	if !ok {
		return nil
	}
	set, ui, err := reader.GetStaticSiteSettings(ctx, name)
	if err != nil {
		return err
	}
	redirects := make([]staticroutes.UIRedirect, 0, len(ui.Redirects))
	for _, r := range ui.Redirects {
		redirects = append(redirects, staticroutes.UIRedirect(r))
	}
	headers := make([]staticroutes.UIHeader, 0, len(ui.Headers))
	for _, h := range ui.Headers {
		headers = append(headers, staticroutes.UIHeader(h))
	}
	rules, blocks, probs := staticroutes.CompileUI(redirects, headers, l)
	if len(probs) > 0 {
		return fmt.Errorf("dashboard rules: %s", probs[0].Message)
	}
	wrapped := staticroutes.Ruleset{StaticRuleset: *rs}
	err = wrapped.ApplyUI(rules, blocks, staticroutes.OptionOverrides{SPA: set.SPAFallback, CleanURLs: set.CleanURLs, TrailingSlash: set.TrailingSlash, NotFound: set.NotFoundPage})
	*rs = wrapped.StaticRuleset
	return err
}
