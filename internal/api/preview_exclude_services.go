package api

import (
	"context"
	"strings"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewSkipDrafts reports the live setting; a missing settings service means off.
func (rt *Router) previewSkipDrafts(ctx context.Context) bool {
	return rt.runtimeSettings != nil && rt.runtimeSettings.Bool(ctx, runtimesettings.KeyPreviewSkipDrafts)
}

func splitExcludeServices(raw string) []string {
	var out []string
	for _, k := range strings.Split(raw, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// withoutExcludedServices drops the services a preview leaves out. It never
// returns an empty map: if every service were excluded the input stays whole.
func withoutExcludedServices(services map[string]spec.Service, raw string) map[string]spec.Service {
	skip := splitExcludeServices(raw)
	if len(skip) == 0 {
		return services
	}
	out := make(map[string]spec.Service, len(services))
	for k, v := range services {
		out[k] = v
	}
	for _, k := range skip {
		delete(out, k)
	}
	if len(out) == 0 {
		return services
	}
	return out
}

// applyPreviewExcludeFields validates the excluded service keys of a policy update.
func applyPreviewExcludeFields(gs *store.GitSource, s *store.PreviewAppSettings, req setPreviewPolicyRequest) string {
	if req.ExcludeServices == nil {
		return ""
	}
	seen := map[string]bool{}
	var keys []string
	for _, k := range *req.ExcludeServices {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		if _, ok := gs.Services[k]; !ok {
			return "exclude_services names unknown service " + k
		}
		seen[k] = true
		keys = append(keys, k)
	}
	if len(keys) > 0 && len(keys) >= len(gs.Services) {
		return "exclude_services cannot leave a preview with no service"
	}
	s.ExcludeServices = strings.Join(keys, ",")
	return ""
}
