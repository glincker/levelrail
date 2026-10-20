package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Digest resolves an image tag to its manifest-list digest using the
// registry's anonymous pull token. It returns "" when the tag does not exist.
func Digest(ctx context.Context, hc *http.Client, image, tag string) (string, error) {
	host, repo, ok := strings.Cut(image, "/")
	if !ok {
		return "", fmt.Errorf("image %q has no registry host", image)
	}
	return digestAt(ctx, hc, "https://"+host, repo, tag)
}

func digestAt(ctx context.Context, hc *http.Client, base, repo, tag string) (string, error) {
	tokReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/token?scope=repository:%s:pull", base, repo), nil)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	tokResp, err := hc.Do(tokReq)
	if err != nil {
		return "", fmt.Errorf("registry token for %s: %w", repo, err)
	}
	defer func() { _ = tokResp.Body.Close() }()
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokResp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decode registry token for %s: %w", repo, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fmt.Sprintf("%s/v2/%s/manifests/%s", base, repo, tag), nil)
	if err != nil {
		return "", fmt.Errorf("build manifest request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("manifest %s:%s: %w", repo, tag, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", nil
	case resp.StatusCode >= 300:
		return "", fmt.Errorf("manifest %s:%s: %s", repo, tag, resp.Status)
	}
	return resp.Header.Get("Docker-Content-Digest"), nil
}
