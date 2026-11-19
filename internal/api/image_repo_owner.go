package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/distribution/reference"

	"github.com/GLINCKER/levelrail/internal/store"
)

// imageRepoClaimer is the atomic ownership table; stores without it fall back to the list scan.
type imageRepoClaimer interface {
	ClaimImageRepo(ctx context.Context, repo, app string) (string, error)
	ImageRepoOwner(ctx context.Context, repo string) (string, error)
}

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
	c, ok := rt.apps.(imageRepoClaimer)
	if !ok {
		return 0, ""
	}
	var owner string
	if claim {
		owner, err = c.ClaimImageRepo(ctx, norm, name)
	} else {
		owner, err = c.ImageRepoOwner(ctx, norm)
	}
	if err != nil {
		return http.StatusInternalServerError, "could not verify the image repo is unused"
	}
	if owner != "" && owner != name {
		return http.StatusConflict, imageRepoTakenMsg(repo, owner)
	}
	return 0, ""
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
