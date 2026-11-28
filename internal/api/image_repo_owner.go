package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/distribution/reference"

	"github.com/glincker/levelrail/internal/deploy"
	"github.com/glincker/levelrail/internal/store"
)

// normalizeImageRepo folds docker.io/library prefixes and case so spellings of one repo compare equal.
func normalizeImageRepo(repo string) string {
	repo = strings.ToLower(strings.TrimSpace(repo))
	named, err := reference.ParseNormalizedNamed(repo)
	if err != nil {
		return repo
	}
	return named.Name()
}

// checkFinalImageRepo is the single ownership gate on the repo a build or spec will
// actually use. claim records ownership atomically; otherwise it only reads. Errors fail closed.
func (rt *Router) checkFinalImageRepo(ctx context.Context, name, repo string, claim bool) (int, string) {
	if repo == "" {
		return 0, ""
	}
	norm := normalizeImageRepo(repo)
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return http.StatusInternalServerError, "could not verify the image repo is unused"
	}
	// A clone still holding the source image only borrows that repo, so it must not count as its owner.
	all = slices.Clone(all)
	for i, o := range all {
		if rt.ownImageRepo(ctx, o.Name, o.Image) == o.Name {
			all[i].Image = ""
		}
	}
	if other := imageRepoOwner(all, name, norm); other != "" {
		return http.StatusConflict, imageRepoTakenMsg(repo, other)
	}
	var owner string
	if claim {
		owner, err = rt.apps.ClaimImageRepo(ctx, norm, name)
	} else {
		owner, err = rt.apps.ImageRepoOwner(ctx, norm)
	}
	if err != nil {
		return http.StatusInternalServerError, "could not verify the image repo is unused"
	}
	if owner != "" && owner != name {
		return http.StatusConflict, imageRepoTakenMsg(repo, owner)
	}
	return 0, ""
}

// checkServiceImageRepo gates and claims the repo a service's image derives for builds.
func (rt *Router) checkServiceImageRepo(ctx context.Context, svc store.DesiredService) (int, string) {
	return rt.checkFinalImageRepo(ctx, svc.Name, rt.ownImageRepo(ctx, svc.Name, svc.Image), true)
}

func imageRepoTakenMsg(repo, other string) string {
	return "image repo " + repo + " is already used by app " + other + ", choose a different image_repo"
}

// imageRepoOwner compares the derived repo of every other app, both sides normalized.
func imageRepoOwner(all []store.DesiredService, name, repo string) string {
	want := normalizeImageRepo(repo)
	for _, o := range all {
		if o.Name != name && normalizeImageRepo(defaultImageRepo(o.Name, o.Image)) == want {
			return o.Name
		}
	}
	return ""
}

// ownImageRepo is the repo this app builds to. An image tagged with a commit SHA whose
// repo another app owns is only a pulled base (a clone), so the app builds to its own name.
func (rt *Router) ownImageRepo(ctx context.Context, name, image string) string {
	repo := defaultImageRepo(name, image)
	if repo == name {
		return repo
	}
	owner, err := rt.apps.ImageRepoOwner(ctx, normalizeImageRepo(repo))
	if err == nil && owner != "" && owner != name {
		return name
	}
	return repo
}

// manualBuildRequestFor is manualBuildRequest with the app's own repo when none was requested.
func (rt *Router) manualBuildRequestFor(ctx context.Context, name string, existing store.DesiredService, req triggerBuildRequest, buildType string) deploy.Request {
	out := manualBuildRequest(name, existing, req, buildType)
	if req.ImageRepo == "" {
		out.ImageRepo = rt.ownImageRepo(ctx, name, existing.Image)
	}
	return out
}

// releaseImageRepoClaim frees the claims a failed create, clone or import took for a new app.
func (rt *Router) releaseImageRepoClaim(ctx context.Context, name string) {
	rel, ok := rt.apps.(interface {
		ReleaseImageRepos(ctx context.Context, app string) error
	})
	if !ok {
		return
	}
	if err := rel.ReleaseImageRepos(context.WithoutCancel(ctx), name); err != nil {
		rt.logger.Warn("api: release image repo claim failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}
