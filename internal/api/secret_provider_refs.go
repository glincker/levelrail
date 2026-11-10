package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secretprovider"
)

const providerRefNeedsRoot = "a secret provider reference (doppler://, infisical:// or op://) can only be set by root, the tier that manages secret providers"

// resolvesThroughProvider mirrors the reconciler's resolver. Without a provider store an op:// value counts, failing closed.
func (rt *Router) resolvesThroughProvider(ctx context.Context, v string) bool {
	if !strings.HasPrefix(v, secretprovider.SchemeDoppler) && !strings.HasPrefix(v, secretprovider.SchemeInfisical) && !strings.HasPrefix(v, secretprovider.SchemeOnePass) {
		return false
	}
	st, ok := rt.secretProviderStore()
	if !ok {
		return true
	}
	live, err := st.ResolvesLive(ctx, v)
	if err != nil {
		rt.logger.Warn("api: provider reference check failed", slog.String("error", err.Error()))
		return true
	}
	return live
}

// allowProviderRefs writes 403 and returns false when a value would read a provider secret and the caller is not root.
// A provider token reads every secret in its scope, so pointing an app at it is a provider-management act.
func (rt *Router) allowProviderRefs(w http.ResponseWriter, r *http.Request, values ...string) bool {
	for _, v := range values {
		if rt.resolvesThroughProvider(r.Context(), v) {
			if rt.callerHasAbility(r, AbilityRoot) {
				return true
			}
			writeError(w, http.StatusForbidden, providerRefNeedsRoot)
			return false
		}
	}
	return true
}
