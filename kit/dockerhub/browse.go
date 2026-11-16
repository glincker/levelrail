package dockerhub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type rawImage struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Variant      string `json:"variant"`
	Digest       string `json:"digest"`
	Size         int64  `json:"size"`
}

// rawTag mirrors the Hub tag object. Documented: name, full_size,
// last_updated, images[].architecture/os/variant/digest. Read when present
// but not documented: digest, tag_last_pushed, images[].size.
type rawTag struct {
	Name          string     `json:"name"`
	Digest        string     `json:"digest"`
	FullSize      int64      `json:"full_size"`
	LastUpdated   string     `json:"last_updated"`
	TagLastPushed string     `json:"tag_last_pushed"`
	Images        []rawImage `json:"images"`
}

func convertTags(raw []rawTag) []Tag {
	out := make([]Tag, 0, len(raw))
	for _, r := range raw {
		t := Tag{Name: r.Name, Digest: r.Digest, FullSize: r.FullSize, LastUpdated: r.LastUpdated, LastPushed: r.TagLastPushed}
		if t.LastPushed == "" {
			t.LastPushed = r.LastUpdated
		}
		seen := map[string]bool{}
		for _, im := range r.Images {
			// Attestation manifests report unknown/unknown; they are not platforms.
			if im.Architecture == "" || im.Architecture == "unknown" || im.OS == "unknown" {
				continue
			}
			t.Images = append(t.Images, Image(im))
			label := im.OS + "/" + im.Architecture
			if im.Variant != "" {
				label += "/" + im.Variant
			}
			if !seen[label] {
				seen[label] = true
				t.Architectures = append(t.Architectures, label)
			}
		}
		out = append(out, t)
	}
	return out
}

// TagPage is one page of tags.
type TagPage struct {
	Count   int   `json:"count"`
	Page    int   `json:"page"`
	HasNext bool  `json:"has_next"`
	Tags    []Tag `json:"tags"`
}

// ListTagsPage lists tags with digest, size, architectures and push time.
// With a token it uses the namespace route that organization tokens need,
// and falls back to the older route for a personal token the namespace
// route refuses. Without a token only public repositories answer.
func (c *Client) ListTagsPage(ctx context.Context, cred Credentials, namespace, repository string, page, pageSize int) (TagPage, error) {
	if namespace == "" {
		namespace = officialNamespace
	}
	if page < 1 {
		page = 1
	}
	q := fmt.Sprintf("?page=%d&page_size=%d", page, pageSize)
	ns, repo := url.PathEscape(namespace), url.PathEscape(repository)
	legacy := fmt.Sprintf("%s/v2/repositories/%s/%s/tags%s", c.baseURL(), ns, repo, q)
	var out tagsResponse
	var err error
	if cred.Present() {
		var bearer string
		if bearer, err = c.hubBearer(ctx, cred); err != nil {
			return TagPage{}, err
		}
		err = c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/v2/namespaces/%s/repositories/%s/tags%s", c.baseURL(), ns, repo, q), bearer, nil, &out)
		if errors.Is(err, ErrForbidden) && cred.Kind() == KindPersonal {
			out = tagsResponse{}
			err = c.doJSON(ctx, http.MethodGet, legacy, bearer, nil, &out)
		}
	} else {
		err = c.doJSON(ctx, http.MethodGet, legacy, "", nil, &out)
	}
	if err != nil {
		return TagPage{}, fmt.Errorf("dockerhub: list tags for %s/%s: %w", namespace, repository, err)
	}
	return TagPage{Count: out.Count, Page: page, HasNext: out.Next != nil && *out.Next != "", Tags: convertTags(out.Results)}, nil
}

// RepoEntry is one repository of a namespace.
type RepoEntry struct {
	FullName    string `json:"full_name"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	IsPrivate   bool   `json:"is_private"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	PullCount   int64  `json:"pull_count"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// RepositoryPage is one page of a namespace listing.
type RepositoryPage struct {
	Count        int         `json:"count"`
	Page         int         `json:"page"`
	HasNext      bool        `json:"has_next"`
	Repositories []RepoEntry `json:"repositories"`
}

type rawRepo struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	IsPrivate   bool   `json:"is_private"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	PullCount   int64  `json:"pull_count"`
	LastUpdated string `json:"last_updated"`
}

// ListRepositories lists a namespace through the namespace route. A token
// adds the private repositories it may see; for an organization token that
// needs the repository list scope, otherwise Docker omits them silently.
func (c *Client) ListRepositories(ctx context.Context, cred Credentials, namespace, nameFilter string, page, pageSize int) (RepositoryPage, error) {
	if namespace == "" {
		return RepositoryPage{}, errors.New("dockerhub: namespace is required")
	}
	if page < 1 {
		page = 1
	}
	q := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}, "ordering": {"-last_updated"}}
	if nameFilter != "" {
		q.Set("name", nameFilter)
	}
	bearer := ""
	if cred.Present() {
		var err error
		if bearer, err = c.hubBearer(ctx, cred); err != nil {
			return RepositoryPage{}, err
		}
	}
	var out struct {
		Count   int       `json:"count"`
		Next    *string   `json:"next"`
		Results []rawRepo `json:"results"`
	}
	target := fmt.Sprintf("%s/v2/namespaces/%s/repositories?%s", c.baseURL(), url.PathEscape(namespace), q.Encode())
	if err := c.doJSON(ctx, http.MethodGet, target, bearer, nil, &out); err != nil {
		return RepositoryPage{}, fmt.Errorf("dockerhub: list repositories of %s: %w", namespace, err)
	}
	res := RepositoryPage{Count: out.Count, Page: page, HasNext: out.Next != nil && *out.Next != "", Repositories: make([]RepoEntry, 0, len(out.Results))}
	for _, r := range out.Results {
		ns := r.Namespace
		if ns == "" {
			ns = namespace
		}
		res.Repositories = append(res.Repositories, RepoEntry{
			FullName: ns + "/" + r.Name, Name: r.Name, Namespace: ns, IsPrivate: r.IsPrivate,
			Description: r.Description, StarCount: r.StarCount, PullCount: r.PullCount, LastUpdated: r.LastUpdated,
		})
	}
	return res, nil
}

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

const manifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"

// Manifest is the digest a tag points at right now.
type Manifest struct {
	Digest    string     `json:"digest"`
	MediaType string     `json:"media_type"`
	Limits    PullLimits `json:"limits"`
}

// ResolveDigest finds the digest of namespace/repository:tag with a HEAD on
// the manifest, which does not count against the pull limit. For a
// multi-architecture image this is the index digest, the one to pin.
func (c *Client) ResolveDigest(ctx context.Context, cred Credentials, namespace, repository, tag string) (Manifest, error) {
	if namespace == "" {
		namespace = officialNamespace
	}
	name := namespace + "/" + repository
	tok, err := c.registryToken(ctx, cred, "repository:"+name+":pull")
	if err != nil {
		return Manifest{}, err
	}
	hdr, err := c.headManifest(ctx, tok.Token, name, tag)
	if err != nil {
		return Manifest{}, fmt.Errorf("dockerhub: resolve %s:%s: %w", name, tag, err)
	}
	digest := strings.TrimSpace(hdr.Get("Docker-Content-Digest"))
	if !digestPattern.MatchString(digest) {
		return Manifest{}, fmt.Errorf("dockerhub: resolve %s:%s: registry returned no valid digest", name, tag)
	}
	lim := ParseRateLimit(hdr)
	lim.Authenticated = cred.Present()
	return Manifest{Digest: digest, MediaType: hdr.Get("Content-Type"), Limits: lim}, nil
}

// headManifest sends HEAD /v2/<name>/manifests/<ref> with a registry bearer.
// A registry answers 401 for a repository the token cannot pull, so the
// caller sees ErrUnauthorized for both a bad token and a missing repo grant.
func (c *Client) headManifest(ctx context.Context, bearer, name, ref string) (http.Header, error) {
	target := fmt.Sprintf("%s/v2/%s/manifests/%s", c.registryURL(), name, url.PathEscape(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", manifestAccept)
	req.Header.Set("Authorization", "Bearer "+bearer)
	return c.send(req, nil)
}
