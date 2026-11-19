package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/distribution/reference"

	"github.com/GLINCKER/levelrail/internal/store"
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
	return rt.checkFinalImageRepo(ctx, svc.Name, defaultImageRepo(svc.Name, svc.Image), true)
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
