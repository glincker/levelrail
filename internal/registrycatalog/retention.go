package registrycatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// manifestAccept lists the manifest types the registry may hold for a tag.
const manifestAccept = "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json"

// TagInfo is one tag with the digest its manifest has and when the image was built.
type TagInfo struct {
	Tag     string
	Digest  string
	Created time.Time
}

type manifestDoc struct {
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

type configDoc struct {
	Created time.Time `json:"created"`
}

// TagInfo reads the digest and build time of repository:tag. Created is zero when the image
// config carries none (an index or a foreign manifest), which sorts as oldest.
func (c *Client) TagInfo(ctx context.Context, baseURL, username, password, repository, tag string) (TagInfo, error) {
	target := baseURL + "/v2/" + escapeRepositoryPath(repository) + "/manifests/" + tag
	resp, err := c.do(ctx, http.MethodGet, target, username, password, manifestAccept)
	if err != nil {
		return TagInfo{}, fmt.Errorf("registrycatalog: manifest %s:%s: %w", repository, tag, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return TagInfo{}, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TagInfo{}, fmt.Errorf("registrycatalog: manifest %s:%s: unexpected status %d", repository, tag, resp.StatusCode)
	}
	info := TagInfo{Tag: tag, Digest: resp.Header.Get("Docker-Content-Digest")}
	var m manifestDoc
	if json.NewDecoder(resp.Body).Decode(&m) != nil || m.Config.Digest == "" {
		return info, nil
	}
	var cfg configDoc
	blob := baseURL + "/v2/" + escapeRepositoryPath(repository) + "/blobs/" + m.Config.Digest
	if err := c.get(ctx, blob, username, password, &cfg); err == nil {
		info.Created = cfg.Created
	}
	return info, nil
}

// DeleteManifest removes the manifest with this digest, which untags every tag pointing at it.
func (c *Client) DeleteManifest(ctx context.Context, baseURL, username, password, repository, digest string) error {
	target := baseURL + "/v2/" + escapeRepositoryPath(repository) + "/manifests/" + digest
	resp, err := c.do(ctx, http.MethodDelete, target, username, password, "")
	if err != nil {
		return fmt.Errorf("registrycatalog: delete manifest %s: %w", digest, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("registrycatalog: delete manifest %s: unexpected status %d", digest, resp.StatusCode)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, target, username, password, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if username == "" && password != "" {
		req.Header.Set("Authorization", "Bearer "+password)
	} else {
		req.SetBasicAuth(username, password)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return c.HTTP.Do(req)
}

// PlanPrune picks the tags to remove so the newest keep tags remain. A tag in inUse is never
// returned, and a digest shared with a kept or in-use tag is never returned either, because
// deleting a manifest untags every tag that points at it. keep <= 0 plans nothing.
func PlanPrune(tags []TagInfo, keep int, inUse map[string]bool) []TagInfo {
	if keep <= 0 || len(tags) <= keep {
		return nil
	}
	sorted := append([]TagInfo(nil), tags...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].Created.Equal(sorted[j].Created) {
			return sorted[i].Created.After(sorted[j].Created)
		}
		return sorted[i].Tag > sorted[j].Tag
	})
	protected := map[string]bool{}
	for i, t := range sorted {
		if i < keep || inUse[t.Tag] {
			protected[t.Digest] = true
		}
	}
	var out []TagInfo
	for _, t := range sorted[keep:] {
		if inUse[t.Tag] || t.Digest == "" || protected[t.Digest] {
			continue
		}
		out = append(out, t)
	}
	return out
}
