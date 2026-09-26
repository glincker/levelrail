package preflight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Image lookup errors a check maps to a status.
var (
	ErrImageNotFound     = errors.New("image not found")
	ErrImageUnauthorized = errors.New("image requires credentials")
	ErrImageRateLimited  = errors.New("registry rate limited")
)

// ImageInfo is what a manifest lookup learned about an image.
type ImageInfo struct {
	SizeBytes int64
}

// ImageInspector resolves an image reference against its registry.
type ImageInspector interface {
	Inspect(ctx context.Context, ref string) (ImageInfo, error)
}

const (
	maxManifestBytes = 1 << 20
	manifestAccept   = "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json"
)

type cachedInfo struct {
	info ImageInfo
	err  error
	at   time.Time
}

// RegistryInspector reads manifests over the registry v2 API with anonymous
// bearer tokens, caching results for TTL.
type RegistryInspector struct {
	Client *http.Client
	TTL    time.Duration

	mu    sync.Mutex
	cache map[string]cachedInfo
}

// Inspect implements ImageInspector.
func (r *RegistryInspector) Inspect(ctx context.Context, ref string) (ImageInfo, error) {
	r.mu.Lock()
	if c, ok := r.cache[ref]; ok && time.Since(c.at) < r.TTL {
		r.mu.Unlock()
		return c.info, c.err
	}
	r.mu.Unlock()

	info, err := r.fetch(ctx, ref)
	if ctx.Err() == nil && (err == nil || errors.Is(err, ErrImageNotFound) || errors.Is(err, ErrImageUnauthorized)) {
		r.mu.Lock()
		if r.cache == nil {
			r.cache = map[string]cachedInfo{}
		}
		r.cache[ref] = cachedInfo{info: info, err: err, at: time.Now()}
		r.mu.Unlock()
	}
	return info, err
}

type imageRef struct{ host, repo, tag string }

func parseImageRef(ref string) imageRef {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i] + ":" + ref[i+1:]
	}
	host, rest := "registry-1.docker.io", ref
	if i := strings.Index(ref, "/"); i >= 0 {
		first := ref[:i]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			host, rest = first, ref[i+1:]
		}
	}
	tag := "latest"
	if i := strings.LastIndex(rest, ":"); i >= 0 && !strings.Contains(rest[i:], "/") {
		rest, tag = rest[:i], rest[i+1:]
	}
	if host == "registry-1.docker.io" && !strings.Contains(rest, "/") {
		rest = "library/" + rest
	}
	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"
	}
	return imageRef{host: host, repo: rest, tag: tag}
}

func (r *RegistryInspector) fetch(ctx context.Context, ref string) (ImageInfo, error) {
	p := parseImageRef(ref)
	scheme := "https"
	if strings.HasPrefix(p.host, "localhost") || strings.HasPrefix(p.host, "127.0.0.1") {
		scheme = "http"
	}
	base := scheme + "://" + p.host + "/v2/" + p.repo + "/manifests/"
	token := ""
	get := func(u string) ([]byte, error) {
		body, ch, err := r.getManifest(ctx, u, token)
		if ch == nil || err != nil {
			return body, err
		}
		tok, terr := r.token(ctx, ch)
		if terr != nil {
			return nil, terr
		}
		token = tok
		body, _, err = r.getManifest(ctx, u, token)
		return body, err
	}
	body, err := get(base + p.tag)
	if err != nil {
		return ImageInfo{}, err
	}
	return manifestSize(body, func(digest string) ([]byte, error) { return get(base + digest) }), nil
}

// tokenChallenge is a registry's 401 bearer challenge.
type tokenChallenge struct{ realm, service, scope string }

func (r *RegistryInspector) getManifest(ctx context.Context, u, token string) ([]byte, *tokenChallenge, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("preflight: build manifest request: %w", err)
	}
	req.Header.Set("Accept", manifestAccept)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("preflight: fetch manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
		if rerr != nil {
			return nil, nil, fmt.Errorf("preflight: read manifest: %w", rerr)
		}
		return b, nil, nil
	case http.StatusUnauthorized:
		if token == "" {
			if ch := parseChallenge(resp.Header.Get("Www-Authenticate")); ch != nil {
				return nil, ch, nil
			}
		}
		return nil, nil, ErrImageUnauthorized
	case http.StatusForbidden:
		return nil, nil, ErrImageUnauthorized
	case http.StatusNotFound:
		return nil, nil, ErrImageNotFound
	case http.StatusTooManyRequests:
		return nil, nil, ErrImageRateLimited
	default:
		return nil, nil, fmt.Errorf("preflight: registry answered %d", resp.StatusCode)
	}
}

func parseChallenge(h string) *tokenChallenge {
	if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return nil
	}
	ch := &tokenChallenge{}
	for _, part := range strings.Split(h[len("bearer "):], ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		switch strings.ToLower(k) {
		case "realm":
			ch.realm = v
		case "service":
			ch.service = v
		case "scope":
			ch.scope = v
		}
	}
	if ch.realm == "" {
		return nil
	}
	return ch
}

func (r *RegistryInspector) token(ctx context.Context, ch *tokenChallenge) (string, error) {
	q := url.Values{}
	if ch.service != "" {
		q.Set("service", ch.service)
	}
	if ch.scope != "" {
		q.Set("scope", ch.scope)
	}
	u := ch.realm
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("preflight: build token request: %w", err)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("preflight: fetch registry token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ErrImageUnauthorized
	case http.StatusTooManyRequests:
		return "", ErrImageRateLimited
	default:
		return "", fmt.Errorf("preflight: token endpoint answered %d", resp.StatusCode)
	}
	var out struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("preflight: decode registry token: %w", err)
	}
	if out.Token != "" {
		return out.Token, nil
	}
	return out.AccessToken, nil
}

type manifestDoc struct {
	Config    struct{ Size int64 }   `json:"config"`
	Layers    []struct{ Size int64 } `json:"layers"`
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	} `json:"manifests"`
}

// manifestSize sums config and layer sizes, following an index to its
// linux/amd64 entry. Anything unparseable yields an unknown (zero) size.
func manifestSize(body []byte, fetchByDigest func(string) ([]byte, error)) ImageInfo {
	var doc manifestDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return ImageInfo{}
	}
	if len(doc.Layers) == 0 && len(doc.Manifests) > 0 {
		digest := doc.Manifests[0].Digest
		for _, m := range doc.Manifests {
			if m.Platform.OS == "linux" && m.Platform.Architecture == "amd64" {
				digest = m.Digest
				break
			}
		}
		inner, err := fetchByDigest(digest)
		if err != nil {
			return ImageInfo{}
		}
		doc = manifestDoc{}
		if err := json.Unmarshal(inner, &doc); err != nil {
			return ImageInfo{}
		}
	}
	size := doc.Config.Size
	for _, l := range doc.Layers {
		size += l.Size
	}
	return ImageInfo{SizeBytes: size}
}
