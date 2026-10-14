package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/scim"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	scimTokenPrefix      = "scim_" //nolint:gosec // token prefix, not a credential
	scimBasePath         = "/api/v1/scim/v2"
	scimMaxBody          = 1 << 20
	scimMaxPage          = 200
	scimTouchEvery       = time.Minute
	auditActorTypeSCIM   = "scim_token"
	envSCIMFailuresPerIP = "APP_SCIM_AUTH_FAILURES_PER_MINUTE"

	defaultSCIMFailuresPerIP = 30
)

// SCIMStore is the persistence the SCIM routes need. *store.DB satisfies it.
type SCIMStore interface {
	CreateSCIMToken(ctx context.Context, t store.SCIMToken) error
	ListSCIMTokens(ctx context.Context) ([]store.SCIMToken, error)
	GetSCIMToken(ctx context.Context, id string) (store.SCIMToken, error)
	ActiveSCIMTokenByHash(ctx context.Context, hash string) (store.SCIMToken, error)
	RevokeSCIMToken(ctx context.Context, id string, now time.Time) error
	RotateSCIMToken(ctx context.Context, oldID string, next store.SCIMToken) error
	TouchSCIMToken(ctx context.Context, id string, now time.Time) error
	ListSCIMUsers(ctx context.Context, attr, value string, offset, limit int) ([]store.SCIMUser, int, error)
	GetSCIMUser(ctx context.Context, userID string) (store.SCIMUser, error)
	SaveSCIMUser(ctx context.Context, u store.SCIMUser, now time.Time) error
	MarkSCIMUserDeleted(ctx context.Context, userID string, now time.Time) error
	GetSCIMGroup(ctx context.Context, id string) (store.SCIMGroup, error)
	ListSCIMGroups(ctx context.Context, attr, value string, offset, limit int) ([]store.SCIMGroup, int, error)
	SaveSCIMGroup(ctx context.Context, g store.SCIMGroup, create bool, changes []store.MemberChange, now time.Time) ([]string, error)
	DeleteSCIMGroup(ctx context.Context, id string) ([]string, error)
	SCIMGroupNamesForUser(ctx context.Context, userID string) ([]string, error)
	SCIMGroupRefsForUser(ctx context.Context, userID string) ([][2]string, error)
	UserDisabled(ctx context.Context, userID string) (bool, error)
	DisableUser(ctx context.Context, userID, reason string, now time.Time) error
	EnableUser(ctx context.Context, userID string) error
	SetSCIMDefaultRole(ctx context.Context, roleID string, now time.Time) error
}

type scimHandler func(w http.ResponseWriter, r *http.Request, tok store.SCIMToken)

func writeSCIM(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", scim.ContentType)
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func writeSCIMError(w http.ResponseWriter, status int, scimType, detail string) {
	writeSCIM(w, status, scim.Error{Schemas: []string{scim.SchemaError}, Status: strconv.Itoa(status), ScimType: scimType, Detail: detail})
}

// requireSCIM authenticates a directory sync request by its bearer token.
// Plain HTTP is refused even with a valid token, which must not travel in clear.
func (rt *Router) requireSCIM(next scimHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rt.scim == nil {
			writeSCIMError(w, http.StatusNotImplemented, "", errSSOUnavailable.Error())
			return
		}
		raw, ok := bearerToken(r)
		if !ok || !strings.HasPrefix(raw, scimTokenPrefix) {
			rt.scimAuthFailed(w, r)
			return
		}
		if !requestIsHTTPS(r) {
			writeSCIMError(w, http.StatusForbidden, "", "SCIM requires https")
			return
		}
		tok, err := rt.scim.ActiveSCIMTokenByHash(r.Context(), hashToken(raw))
		if errors.Is(err, store.ErrSCIMTokenNotFound) {
			rt.scimAuthFailed(w, r)
			return
		}
		if err != nil {
			rt.logger.Error("api: scim token lookup failed", slog.String("error", err.Error()))
			writeSCIMError(w, http.StatusInternalServerError, "", "internal error")
			return
		}
		now := time.Now()
		if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) > scimTouchEvery {
			if err := rt.scim.TouchSCIMToken(r.Context(), tok.ID, now); err != nil {
				rt.logger.Warn("api: touch scim token failed", slog.String("token_id", tok.ID), slog.String("error", err.Error()))
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, scimMaxBody)
		next(w, r, tok)
	}
}

func (rt *Router) scimAuthFailed(w http.ResponseWriter, r *http.Request) {
	if ok, retry := rt.scimLimiter.allow(clientIP(r)); !ok {
		writeRateLimited(w, retry)
		return
	}
	writeSCIMError(w, http.StatusUnauthorized, "", "a valid SCIM bearer token is required")
}

func (rt *Router) auditSCIM(r *http.Request, tok store.SCIMToken, action, target string, status int) {
	rt.auditSignIn(r.Context(), r, signInActor{kind: auditActorTypeSCIM, id: tok.ID, name: tok.Name}, action, scimBasePath+target, status)
}

// scimBase is the absolute SCIM base URL for meta.location, or the path alone without a dashboard URL.
func (rt *Router) scimBase(ctx context.Context) string {
	if base, err := rt.samlBaseURL(ctx); err == nil {
		return base + scimBasePath
	}
	return scimBasePath
}

func scimPage(r *http.Request) (offset, limit, start int) {
	startIndex, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	start, size := scim.Page(startIndex, count, scimMaxPage)
	return start - 1, size, start
}
