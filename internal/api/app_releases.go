package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

// releaseReader is satisfied by *store.DB; a Store without it lists nothing.
type releaseReader interface {
	ListReleasesByApp(ctx context.Context, app string) ([]store.Release, error)
	GetEnvironmentBinding(ctx context.Context, app string) (*store.EnvironmentBinding, error)
	ListBindingHistory(ctx context.Context, app string) ([]store.BindingHistoryEntry, error)
}

// releaseResource is the public view of a release. It carries no config
// snapshot, so nothing that could hold a secret leaves the store.
type releaseResource struct {
	ID              string `json:"id"`
	ImageRef        string `json:"image_ref"`
	Digest          string `json:"digest"`
	ShortDigest     string `json:"short_digest"`
	SourceAttemptID string `json:"source_attempt_id"`
	CreatedAt       string `json:"created_at"`
	SourceReason    string `json:"source_reason"`
}

// bindingResource is one retarget of the app's binding.
type bindingResource struct {
	ReleaseID string `json:"release_id"`
	At        string `json:"at"`
	Reason    string `json:"reason"`
	Actor     string `json:"actor"`
}

type appReleasesResource struct {
	Releases         []releaseResource `json:"releases"`
	CurrentReleaseID string            `json:"current_release_id"`
	// Bindings is the binding history, newest first; omitted only while off (shadow and on both return it).
	Bindings []bindingResource `json:"bindings,omitempty"`
}

// handleListAppReleases handles GET /api/v1/apps/{name}/releases, a read
// only view of the shadow release records, newest first. It answers an empty
// list while APP_RELEASE_BINDINGS is off.
func (rt *Router) handleListAppReleases(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	if _, err := rt.apps.GetDesiredService(ctx, name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list releases: load app failed", err, slog.String("name", name))
		return
	}
	out := appReleasesResource{Releases: []releaseResource{}}
	rd, ok := rt.apps.(releaseReader)
	if !ok || !release.ModeFromEnv().Writes() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	rels, err := rd.ListReleasesByApp(ctx, name)
	if err != nil {
		rt.internalError(w, "api: list releases failed", err, slog.String("name", name))
		return
	}
	hist, err := rd.ListBindingHistory(ctx, name)
	if err != nil {
		rt.internalError(w, "api: list release history failed", err, slog.String("name", name))
		return
	}
	reasons := make(map[string]string, len(hist))
	for _, h := range hist {
		if _, seen := reasons[h.ReleaseID]; !seen {
			reasons[h.ReleaseID] = h.Reason
		}
	}
	for i := len(hist) - 1; i >= 0; i-- {
		h := hist[i]
		out.Bindings = append(out.Bindings, bindingResource{ReleaseID: h.ReleaseID, At: h.ChangedAt.UTC().Format(time.RFC3339), Reason: h.Reason, Actor: h.ChangedBy})
	}
	for _, rel := range rels {
		digest := releaseDigest(rel.ImageRef)
		out.Releases = append(out.Releases, releaseResource{
			ID: rel.ID, ImageRef: rel.ImageRef, Digest: digest, ShortDigest: shortDigest(digest),
			SourceAttemptID: rel.SourceDeployAttemptID, CreatedAt: rel.CreatedAt.UTC().Format(time.RFC3339),
			SourceReason: reasons[rel.ID],
		})
	}
	if b, err := rd.GetEnvironmentBinding(ctx, name); err == nil && b != nil {
		out.CurrentReleaseID = b.ReleaseID
	}
	writeJSON(w, http.StatusOK, out)
}

func releaseDigest(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		return ref[i+1:]
	}
	return ""
}

// shortDigest keeps the algorithm prefix and the first 12 hex characters.
func shortDigest(d string) string {
	algo, hexPart, found := strings.Cut(d, ":")
	if !found {
		hexPart, algo = d, ""
	}
	if len(hexPart) > 12 {
		hexPart = hexPart[:12]
	}
	if algo == "" {
		return hexPart
	}
	return algo + ":" + hexPart
}
