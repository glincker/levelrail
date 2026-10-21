package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	// envMaxBodyBytes raises or lowers the backstop for every API request body.
	envMaxBodyBytes = "APP_API_MAX_BODY_BYTES"
	// defaultMaxBodyBytes sits above the largest per-handler cap (8 MiB).
	defaultMaxBodyBytes int64 = 16 << 20
	// preAuthMaxBodyBytes bounds routes an anonymous caller can reach.
	preAuthMaxBodyBytes int64 = 1 << 20
)

// bodyLimitMiddleware caps request bodies before any handler decodes them, so
// an unauthenticated client cannot buffer an unbounded JSON string.
func (rt *Router) bodyLimitMiddleware(next http.Handler) http.Handler {
	backstop := defaultMaxBodyBytes
	if v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(envMaxBodyBytes)), 10, 64); err == nil && v > 0 {
		backstop = v
	}
	enginePrefix := ""
	if rt.authEngine != nil {
		enginePrefix = rt.authEngine.Prefix()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGitPushUpload(r.URL.Path) || isSourceUpload(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			limit := backstop
			if isPreAuthPath(r.URL.Path, enginePrefix) && preAuthMaxBodyBytes < limit {
				limit = preAuthMaxBodyBytes
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

// isGitPushUpload is the one route whose body (a pack) the handler bounds itself, after auth.
func isGitPushUpload(path string) bool {
	rest, ok := strings.CutPrefix(path, gitPushRoutePrefix)
	if !ok {
		return false
	}
	repo, tail, ok := strings.Cut(rest, "/")
	return ok && repo != "" && tail == gitReceivePackService
}

// isSourceUpload is the route whose archive body the handler bounds itself with APP_UPLOAD_MAX_BYTES, after auth.
func isSourceUpload(r *http.Request) bool {
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/apps/")
	if !ok || r.Method != http.MethodPost {
		return false
	}
	app, tail, ok := strings.Cut(rest, "/")
	return ok && app != "" && tail == "uploads"
}

func isPreAuthPath(path, enginePrefix string) bool {
	switch {
	case strings.HasPrefix(path, "/api/v1/auth/"),
		strings.HasPrefix(path, "/api/v1/invites/accept"),
		strings.HasPrefix(path, "/api/v1/team-invites/accept"):
		return true
	}
	return enginePrefix != "" && strings.HasPrefix(path, enginePrefix+"/")
}
