package api

import (
	"net/http"
	"strings"
)

const (
	// preAuthMaxBodyBytes bounds routes an anonymous caller can reach.
	preAuthMaxBodyBytes int64 = 1 << 20
)

// bodyLimitMiddleware caps request bodies before any handler decodes them, so
// an unauthenticated client cannot buffer an unbounded JSON string.
func (rt *Router) bodyLimitMiddleware(next http.Handler) http.Handler {
	enginePrefix := ""
	if rt.authEngine != nil {
		enginePrefix = rt.authEngine.Prefix()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGitPushUpload(r.URL.Path) || isSourceUpload(r) || isAssetObjectUpload(r) || isVolumeImport(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			limit := rt.maxBodyBytes(r.Context())
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

// isAssetObjectUpload streams to a temp file and is capped by assets.upload_max_bytes after auth, so the generic backstop must not apply.
func isAssetObjectUpload(r *http.Request) bool {
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/assets/buckets/")
	if !ok || r.Method != http.MethodPut {
		return false
	}
	id, tail, ok := strings.Cut(rest, "/")
	return ok && id != "" && tail == "objects"
}

// isVolumeImport streams a volume archive that the handler bounds with volumes.transfer_max_bytes after auth.
func isVolumeImport(r *http.Request) bool {
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/apps/")
	if !ok || r.Method != http.MethodPost {
		return false
	}
	parts := strings.Split(rest, "/")
	return len(parts) == 4 && parts[0] != "" && parts[1] == "volumes" && parts[2] != "" && parts[3] == "import"
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
