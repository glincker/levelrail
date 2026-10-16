package deploytrace

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Bytes renders n as a short human size.
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// EnvSummary lists env var names, tagging non-plain sources. Values are never
// included: a secret, vault or database reference must not reach a log.
func EnvSummary(plain []string, tagged map[string]string) string {
	names := make([]string, 0, len(plain)+len(tagged))
	for _, k := range plain {
		if _, dup := tagged[k]; !dup {
			names = append(names, k)
		}
	}
	for k := range tagged {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "no env vars"
	}
	parts := make([]string, len(names))
	for i, k := range names {
		parts[i] = k
		if tag, ok := tagged[k]; ok {
			parts[i] = k + " (" + tag + ", masked)"
		}
	}
	return fmt.Sprintf("%d env var(s): %s", len(names), strings.Join(parts, ", "))
}

// ConfigStep records what a deploy asks the node to run: env names (masked by
// source), port, volumes and resource limits, never values.
func ConfigStep(ctx context.Context, tr *Tracer, svc store.DesiredService) {
	keys := make([]string, 0, len(svc.Env))
	for k := range svc.Env {
		keys = append(keys, k)
	}
	tagged := map[string]string{}
	for _, s := range svc.SecretEnv {
		tagged[s.Name] = "secret"
	}
	for k := range svc.DatabaseEnv {
		tagged[k] = "database"
	}
	for k := range svc.VaultEnv {
		tagged[k] = "vault"
	}
	parts := []string{EnvSummary(keys, tagged), fmt.Sprintf("port %d", svc.Port), fmt.Sprintf("%d volume(s)", len(svc.Volumes))}
	if r := svc.Resources; r != nil && (r.MemoryBytes > 0 || r.NanoCPUs > 0) {
		parts = append(parts, fmt.Sprintf("limits memory %s, cpu %.2f", Bytes(r.MemoryBytes), float64(r.NanoCPUs)/1e9))
	} else {
		parts = append(parts, "no resource limits")
	}
	tr.Note(ctx, StepConfigValidate, store.TraceStatusDone, "", strings.Join(parts, "; "))
}
