package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envPreviewMemory    = "APP_PREVIEW_MEMORY"
	envPreviewCPU       = "APP_PREVIEW_CPU"
	envPreviewIdleSleep = "APP_PREVIEW_IDLE_SLEEP_MINUTES"

	defaultPreviewMemory    = "256Mi"
	defaultPreviewCPU       = 0.25
	defaultPreviewIdleSleep = 30

	previewGateSecretPrefix = "preview-gate/"
	previewGateSecretKey    = "password"
)

// previewResourceLimits is the memory and cpu ceiling for an app's previews:
// the app's own override, else the platform default (APP_PREVIEW_MEMORY,
// APP_PREVIEW_CPU), which is deliberately smaller than a production app.
func previewResourceLimits(s store.PreviewAppSettings) (memory string, cpu float64) {
	memory, cpu = s.MemoryLimit, s.CPULimit
	if memory == "" {
		memory = os.Getenv(envPreviewMemory)
	}
	if memory == "" {
		memory = defaultPreviewMemory
	}
	if cpu <= 0 {
		if v, err := strconv.ParseFloat(os.Getenv(envPreviewCPU), 64); err == nil && v > 0 {
			cpu = v
		} else {
			cpu = defaultPreviewCPU
		}
	}
	return memory, cpu
}

// previewIdleSleepMinutes is how long a preview may see no requests before it
// sleeps; 0 means never.
func previewIdleSleepMinutes(s store.PreviewAppSettings) int {
	if s.IdleSleepMinutes > 0 {
		return s.IdleSleepMinutes
	}
	if v, err := strconv.Atoi(os.Getenv(envPreviewIdleSleep)); err == nil && v >= 0 {
		if v > 0 && v < minSleepIdleMinutes {
			return minSleepIdleMinutes
		}
		return v
	}
	return defaultPreviewIdleSleep
}

// previewMemoryBytes parses "256Mi" style sizes; 0 means unparseable.
func previewMemoryBytes(s string) int64 {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		mult   int64
	}{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40},
		{"K", 1000}, {"M", 1000 * 1000}, {"G", 1000 * 1000 * 1000},
		{"k", 1000}, {"m", 1000 * 1000}, {"g", 1000 * 1000 * 1000},
	}
	mult := int64(1)
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			s, mult = strings.TrimSuffix(s, u.suffix), u.mult
			break
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return int64(n * float64(mult))
}

// clampPreviewResources caps svc at the preview limits. A production limit
// that is already smaller is kept, so a preview never asks for more than the
// app does.
func clampPreviewResources(svc *spec.Service, memory string, cpu float64) {
	r := spec.Resources{}
	if svc.Resources != nil {
		r = *svc.Resources
	}
	limit := previewMemoryBytes(memory)
	cur := previewMemoryBytes(r.Memory)
	if r.Memory == "" || cur == 0 || (limit > 0 && cur > limit) {
		r.Memory = memory
		r.SwapMemory = ""
	}
	if r.CPU <= 0 || r.CPU > cpu {
		r.CPU = cpu
	}
	svc.Resources = &r
}

var databaseEnvKey = regexp.MustCompile(`(?i)(^|_)(DATABASE|DB|POSTGRES|POSTGRESQL|PG|MYSQL|MARIADB|MONGO|MONGODB|REDIS|DSN)(_|$)`)

// isDatabaseEnvKey reports whether an env var name looks like a database
// connection setting, the class a preview must not inherit from production.
func isDatabaseEnvKey(key string) bool {
	return databaseEnvKey.MatchString(key)
}

// dropDatabaseEnv removes database connection variables from env, plus the
// attachment variable production uses, so a preview starts with no route to
// production data unless a preview override adds one back.
func dropDatabaseEnv(env map[string]spec.EnvVar, attachmentVar string) map[string]spec.EnvVar {
	if len(env) == 0 {
		return env
	}
	out := make(map[string]spec.EnvVar, len(env))
	for k, v := range env {
		if v.From != "" || isDatabaseEnvKey(k) || (attachmentVar != "" && k == attachmentVar) {
			continue
		}
		out[k] = v
	}
	return out
}

// previewInheritsEnv reports whether a preview built for this pull request
// may carry the app's environment at all. A fork never does unless the app
// opted in, since fork code is untrusted.
func previewInheritsEnv(fork bool, s store.PreviewAppSettings) bool {
	return !fork || s.AllowForkSecrets
}

// applyPreviewPolicy shapes svc for a preview: environment inheritance,
// database strategy and resource ceiling. Preview overrides are applied by
// the caller afterwards so they always win.
func applyPreviewPolicy(svc *spec.Service, vars previewEnvVars, attachmentVar string) {
	if !previewInheritsEnv(vars.Fork, vars.Policy) {
		svc.Env = nil
	} else if vars.Policy.DatabaseStrategy != store.PreviewDatabaseStrategyShared {
		svc.Env = dropDatabaseEnv(svc.Env, attachmentVar)
	}
	memory, cpu := previewResourceLimits(vars.Policy)
	clampPreviewResources(svc, memory, cpu)
}

// previewHost is the hostname a preview is served on. Under the apps base
// domain it is one label (<app>-pr-<n>.<base>) so the base domain's wildcard
// record covers it; otherwise it falls back to pr-<n>.<app>.<primary domain>.
func (rt *Router) previewHost(ctx context.Context, appName string, prNumber int) string {
	if base := rt.appsBaseDomain(ctx); base != "" {
		return appBaseHost(previewAppName(appName, prNumber), base)
	}
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		rt.logger.Error("api: preview domain: load ingress settings failed", slog.String("error", err.Error()))
		return ""
	}
	if settings.PrimaryDomain == "" {
		return ""
	}
	return fmt.Sprintf("pr-%d.%s.%s", prNumber, ingress.SanitizeDNSLabel(appName), settings.PrimaryDomain)
}

// previewDNS writes the record for a preview host when automatic DNS applies:
// skipped under a wildcard-covered base domain or when automation is off. The
// outcome is returned for the log; DNS is never a deploy failure.
func (rt *Router) previewDNS(ctx context.Context, previewName, host string) domainDNSResult {
	if host == "" {
		return domainDNSResult{DNS: dnsResultSkipped}
	}
	policy := rt.effectiveAutomation(ctx, nil, nil)
	base := rt.appsBaseDomain(ctx)
	oneLabelUnderBase := base != "" && strings.HasSuffix(host, "."+base) && strings.Count(host, ".") == strings.Count(base, ".")+1
	if oneLabelUnderBase && policy.WildcardForBaseDomain {
		return domainDNSResult{Domain: host, DNS: dnsResultSkipped, Message: "covered by the wildcard record for the apps base domain"}
	}
	if !policy.AutoDNS {
		return domainDNSResult{Domain: host, DNS: dnsResultSkipped, Message: "automatic DNS is off"}
	}
	res, _ := rt.applyDomainDNS(ctx, nil, previewName, host, dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	return res
}

// removePreviewDNS deletes the DNS records created for the preview's domain
// and reports a failure when any tracked record is still present afterwards.
func (rt *Router) removePreviewDNS(ctx context.Context, preview store.PreviewEnvironment) []string {
	if preview.Domain == "" {
		return nil
	}
	rt.removeManagedDNS(ctx, nil, preview.PreviewAppID, preview.Domain)
	ms := rt.managedDNS()
	if ms == nil {
		return nil
	}
	left, err := ms.ListManagedDNSRecords(ctx, preview.Domain)
	if err != nil {
		rt.logger.Error("api: preview teardown: check dns records failed", slog.String("error", err.Error()), slog.String("domain", preview.Domain))
		return []string{"dns:" + preview.Domain}
	}
	if len(left) > 0 {
		return []string{"dns:" + preview.Domain}
	}
	return nil
}

// secureReachablePreview applies the exposure policy to a deployed preview
// domain: hidden from search engines unless the app allows indexing, and
// behind basic auth when the app's gate is on.
func (rt *Router) secureReachablePreview(ctx context.Context, appName, domain string, s store.PreviewAppSettings) (notes []string) {
	if domain == "" {
		return nil
	}
	if err := rt.domainSearchVisibility.SetDomainHidden(ctx, domain, !s.AllowIndexing); err != nil {
		rt.logger.Error("api: preview search visibility failed", slog.String("error", err.Error()), slog.String("domain", domain))
		notes = append(notes, "could not hide the preview from search engines")
	}
	if !s.GateBasicAuth {
		return notes
	}
	if err := rt.gatePreviewDomain(ctx, appName, domain, s); err != nil {
		rt.logger.Error("api: preview basic auth gate failed", slog.String("error", err.Error()), slog.String("domain", domain))
		notes = append(notes, "the basic auth gate could not be applied")
	}
	return notes
}

func (rt *Router) gatePreviewDomain(ctx context.Context, appName, domain string, s store.PreviewAppSettings) error {
	if rt.secrets == nil || rt.domainBasicAuthSecrets == nil || rt.domainBasicAuth == nil {
		return fmt.Errorf("basic auth needs a master key on this control plane")
	}
	if s.GateUsername == "" {
		return fmt.Errorf("no gate username is set for %q", appName)
	}
	password, err := rt.secrets.Resolve(ctx, previewGateSecretPrefix+appName, previewGateSecretKey)
	if err != nil || password == "" {
		return fmt.Errorf("no gate password is set for %q", appName)
	}
	key := store.DomainBasicAuthSecretsKey(domain)
	if err := rt.domainBasicAuthSecrets.SetValue(ctx, key, store.DomainBasicAuthPasswordEnvKey, password); err != nil {
		return fmt.Errorf("store gate password: %w", err)
	}
	if err := rt.domainBasicAuth.SetDomainBasicAuth(ctx, domain, s.GateUsername); err != nil {
		return fmt.Errorf("enable gate: %w", err)
	}
	return nil
}

// sleepIdlePreview enables sleep-when-idle on every service of a preview.
func (rt *Router) sleepIdlePreview(ctx context.Context, previewName string, s store.PreviewAppSettings) {
	minutes := previewIdleSleepMinutes(s)
	if minutes <= 0 || rt.appSleep == nil {
		return
	}
	names := []string{previewName}
	if app, err := rt.appGroups.GetAppByName(ctx, previewName); err == nil {
		if svcs, lerr := rt.appGroups.ListServicesByApp(ctx, app.ID); lerr == nil && len(svcs) > 0 {
			names = names[:0]
			for _, svc := range svcs {
				names = append(names, svc.Name)
			}
		}
	}
	now := time.Now().UTC()
	for _, n := range names {
		if err := rt.appSleep.SetAppSleepIdle(ctx, n, minutes, now); err != nil {
			rt.logger.Warn("api: preview idle sleep failed", slog.String("error", err.Error()), slog.String("preview_app", n))
		}
	}
}
