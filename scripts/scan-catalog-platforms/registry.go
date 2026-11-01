package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const manifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"

// maxBody caps any registry response we read, manifests and configs are small.
const maxBody = 4 << 20

type imageRef struct {
	Host, Repo, Reference string
}

// parseRef maps a compose image string onto a registry host, repository and tag or digest.
func parseRef(ref string) (imageRef, error) {
	if ref == "" {
		return imageRef{}, errors.New("empty image reference")
	}
	reference := "latest"
	name := ref
	if i := strings.Index(name, "@"); i >= 0 {
		reference = name[i+1:]
		name = name[:i]
	} else if c, sl := strings.LastIndex(name, ":"), strings.LastIndex(name, "/"); c > sl {
		reference = name[c+1:]
		name = name[:c]
	}
	host := "registry-1.docker.io"
	repo := name
	if i := strings.Index(name, "/"); i >= 0 {
		first := name[:i]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			host, repo = first, name[i+1:]
			if first == "docker.io" || first == "index.docker.io" {
				host = "registry-1.docker.io"
			}
		}
	}
	if host == "registry-1.docker.io" && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return imageRef{Host: host, Repo: repo, Reference: reference}, nil
}

// registryFetcher lists platforms through the registry HTTP API with the
// anonymous bearer flow; no container tooling is involved.
func registryFetcher(client *http.Client) Fetcher {
	return func(ctx context.Context, ref string) ([]string, error) {
		r, err := parseRef(ref)
		if err != nil {
			return nil, err
		}
		body, finalURL, tok, err := get(ctx, client, fmt.Sprintf("https://%s/v2/%s/manifests/%s", r.Host, r.Repo, r.Reference), manifestAccept, "")
		if err != nil {
			return nil, err
		}
		if plats, ok := platformsFromRaw(body); ok {
			return plats, nil
		}
		var one struct {
			Config struct {
				Digest string `json:"digest"`
			} `json:"config"`
		}
		if err := json.Unmarshal(body, &one); err != nil || one.Config.Digest == "" {
			return nil, errors.New("unsupported manifest format")
		}
		base := finalURL.Scheme + "://" + finalURL.Host
		cfgBody, _, _, err := get(ctx, client, fmt.Sprintf("%s/v2/%s/blobs/%s", base, r.Repo, one.Config.Digest), "", tok)
		if err != nil {
			return nil, err
		}
		var cfg struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		}
		if err := json.Unmarshal(cfgBody, &cfg); err != nil || cfg.Architecture == "" {
			return nil, errors.New("no architecture in image config")
		}
		return []string{cfg.OS + "/" + cfg.Architecture}, nil
	}
}

// get fetches a URL, answering one Bearer challenge with an anonymous token.
// It returns the body, the post-redirect URL and the token it used.
func get(ctx context.Context, c *http.Client, rawURL, accept, token string) ([]byte, *url.URL, string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, nil, "", err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := c.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, "", ctx.Err()
			}
			return nil, nil, "", fmt.Errorf("request failed: %w", err)
		}
		data, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		_ = resp.Body.Close()
		final := resp.Request.URL
		switch {
		case resp.StatusCode == http.StatusOK:
			if rerr != nil {
				return nil, nil, "", rerr
			}
			return data, final, token, nil
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, nil, "", errRateLimited
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			tok, terr := anonymousToken(ctx, c, resp.Header.Get("Www-Authenticate"))
			if terr != nil {
				return nil, nil, "", terr
			}
			token, rawURL = tok, final.String()
		case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
			return nil, nil, "", fmt.Errorf("access denied (HTTP %d), image is private or missing", resp.StatusCode)
		case resp.StatusCode == http.StatusNotFound:
			return nil, nil, "", errors.New("not found (HTTP 404)")
		default:
			return nil, nil, "", fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
		}
	}
	return nil, nil, "", errors.New("unreachable")
}

// anonymousToken follows a `Bearer realm=...,service=...,scope=...` challenge.
func anonymousToken(ctx context.Context, c *http.Client, challenge string) (string, error) {
	rest, ok := strings.CutPrefix(challenge, "Bearer ")
	if !ok {
		return "", errors.New("registry requires non-bearer auth")
	}
	params := map[string]string{}
	for _, part := range splitChallenge(rest) {
		if k, v, found := strings.Cut(part, "="); found {
			params[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || realm.Scheme != "https" {
		return "", errors.New("bad token realm in challenge")
	}
	q := realm.Query()
	if params["service"] != "" {
		q.Set("service", params["service"])
	}
	if params["scope"] != "" {
		q.Set("scope", params["scope"])
	}
	realm.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token denied (HTTP %d), image is private or missing", resp.StatusCode)
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&tok); err != nil {
		return "", err
	}
	if tok.Token != "" {
		return tok.Token, nil
	}
	if tok.AccessToken != "" {
		return tok.AccessToken, nil
	}
	return "", errors.New("empty token")
}

// splitChallenge splits on commas outside quotes (scope values contain commas).
func splitChallenge(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
			cur.WriteRune(r)
		case r == ',' && !inQ:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, cur.String())
}
