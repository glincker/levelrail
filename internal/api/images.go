package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/distribution/reference"
)

// imageResource is GET /api/v1/apps/{name}/images's wire shape: one
// locally-present tag under the app's repo, reusing exactly what
// docker.ImageInfo already carries (internal/docker/runtime.go), no
// invented fields.
type imageResource struct {
	Tag       string    `json:"tag"`
	CreatedAt time.Time `json:"created_at"`
}

func toImageResource(i docker.ImageInfo) imageResource {
	return imageResource{Tag: i.Tag, CreatedAt: i.CreatedAt}
}

// repoFromImage derives the repo portion of a full image reference
// (e.g. "ghcr.io/you/app:abc123" -> "ghcr.io/you/app"), the same
// convention docker.Client.ListImages' reference argument expects
// (internal/docker/client.go). Delegating to
// github.com/distribution/reference (already an indirect dependency of
// this module via docker.Client and moby/buildkit, see go.mod) instead
// of a hand-rolled split: a naive "split on the first/only colon" breaks
// the moment a registry host itself carries a port
// ("registry.example.com:5000/app:latest"), because that colon comes
// before the last "/" too. reference.Parse applies Docker's own
// grammar, which splits on the last colon that occurs after the last
// slash, exactly the rule this function needs.
//
// A parse failure (or an empty string) falls back to returning image
// unchanged rather than erroring: this backs an optional "suggest
// previously-built tags" dropdown, not a validation gate, so an
// unparseable current image must never turn into a 500 for the whole
// endpoint. ListImages simply won't find anything for a repo string
// that doesn't correspond to a real image, which degrades to an empty
// list, the same outcome as no ImageLister being configured at all.
func repoFromImage(image string) string {
	if image == "" {
		return ""
	}
	ref, err := reference.Parse(image)
	if err != nil {
		return image
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return image
	}
	return named.Name()
}

// handleListImages handles GET /api/v1/apps/{name}/images: every
// locally-present tag under the app's current image's repo, for the
// deploy trigger form (web/src/components/DeployTriggerForm.tsx) to
// offer as a dropdown instead of forcing an operator to hand-type a
// full image reference every time. Always 200 when the app exists: a
// missing ImageLister or a ListImages failure both degrade to an empty
// list rather than an error response, the same "absence is a reportable
// empty state, never a 5xx" shape handleSystemStatus's DockerConnected
// field already follows for an optional signal.
func (rt *Router) handleListImages(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: list images: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if rt.images == nil {
		writeJSON(w, http.StatusOK, []imageResource{})
		return
	}

	repo := repoFromImage(svc.Image)
	images, err := rt.images.ListImages(r.Context(), repo)
	if err != nil {
		// A registry/daemon hiccup degrades to "no suggestions" instead
		// of blocking the deploy trigger form's core function (typing an
		// image tag manually must always keep working, see ImageLister's
		// doc comment on router.go).
		rt.logger.Error("api: list images failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo", repo))
		writeJSON(w, http.StatusOK, []imageResource{})
		return
	}

	out := make([]imageResource, 0, len(images))
	for _, img := range images {
		out = append(out, toImageResource(img))
	}
	writeJSON(w, http.StatusOK, out)
}
