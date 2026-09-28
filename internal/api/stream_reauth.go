package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// streamRecheckInterval is how often a long-lived stream re-evaluates its
// caller. It is a var so tests can shorten it.
var streamRecheckInterval = 30 * time.Second

// stillAuthorized re-runs the route gate for an already-authenticated
// stream: session or token still valid, and the ability plus IAM policies
// still allow the resource. It fails closed on any lookup error.
func (rt *Router) stillAuthorized(ctx context.Context, r *http.Request, required, resource string) bool {
	principalType, principalID, abilities, err := rt.resolveCaller(ctx, r)
	if err != nil {
		if !errors.Is(err, errCallerGone) {
			rt.logger.Warn("api: stream re-authorization lookup failed", slog.String("error", err.Error()))
		}
		return false
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, principalType, principalID)
	if err != nil {
		rt.logger.Warn("api: stream re-authorization policy lookup failed", slog.String("error", err.Error()))
		return false
	}
	return authorizeResource(abilities, policies, required, resource)
}

var errCallerGone = errors.New("caller is no longer valid")

func (rt *Router) resolveCaller(ctx context.Context, r *http.Request) (principalType, principalID string, abilities []string, err error) {
	if userID, ok := rt.currentSessionUserID(r); ok {
		user, uerr := rt.auth.GetUserByID(ctx, userID)
		if errors.Is(uerr, store.ErrUserNotFound) {
			return "", "", nil, errCallerGone
		}
		if uerr != nil {
			return "", "", nil, fmt.Errorf("load session user: %w", uerr)
		}
		return store.PrincipalTypeUser, userID, user.Abilities, nil
	}
	token, ok := bearerToken(r)
	if !ok {
		return "", "", nil, errCallerGone
	}
	rec, terr := rt.tokens.GetAPITokenByHash(ctx, hashToken(token))
	if errors.Is(terr, store.ErrAPITokenNotFound) {
		return "", "", nil, errCallerGone
	}
	if terr != nil {
		return "", "", nil, fmt.Errorf("load token: %w", terr)
	}
	if rec.RevokedAt != nil || (rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt)) {
		return "", "", nil, errCallerGone
	}
	return store.PrincipalTypeToken, rec.ID, rec.Abilities, nil
}

// watchAuthorization re-checks r's caller every streamRecheckInterval and
// calls revoked once when the gate stops passing. The returned stop ends
// the watcher and must be called when the stream ends.
func (rt *Router) watchAuthorization(r *http.Request, required string, resourceFn func(*http.Request) string, revoked func()) (stop func()) {
	probe := r.Clone(context.WithoutCancel(r.Context()))
	resource := resourceFn(r)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(streamRecheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if !rt.stillAuthorized(probe.Context(), probe, required, resource) {
					revoked()
					return
				}
			}
		}
	}()
	return func() {
		select {
		case <-done:
		default:
			close(done)
		}
	}
}

// withStreamReauth wraps a streaming handler so its request context is
// cancelled as soon as the caller loses read access mid-stream. Every
// streaming route is read-tier; the terminal calls watchAuthorization
// directly with its own ability.
func (rt *Router) withStreamReauth(resourceFn func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		stop := rt.watchAuthorization(r, AbilityRead, resourceFn, cancel)
		defer stop()
		next(w, r.WithContext(ctx))
	}
}
