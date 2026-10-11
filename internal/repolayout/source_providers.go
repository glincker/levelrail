package repolayout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func decodeJSON(b []byte, out any) error {
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

const maxListPages = 40

type githubSource struct {
	c                     *http.Client
	api, repo, ref, token string
}

func (s *githubSource) headers(accept string) map[string]string {
	h := map[string]string{"Accept": accept, "X-GitHub-Api-Version": "2022-11-28"}
	if s.token != "" {
		h["Authorization"] = "Bearer " + s.token
	}
	return h
}

func (s *githubSource) Paths(ctx context.Context) ([]string, bool, error) {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	refURL := s.api + "/repos/" + escapePath(s.repo) + "/git/ref/heads/" + escapePath(s.ref)
	if err := getJSON(ctx, s.c, refURL, s.headers("application/vnd.github+json"), &ref); err != nil {
		return nil, false, err
	}
	var tree struct {
		Truncated bool `json:"truncated"`
		Tree      []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	treeURL := s.api + "/repos/" + escapePath(s.repo) + "/git/trees/" + url.PathEscape(ref.Object.SHA) + "?recursive=1"
	if err := getJSON(ctx, s.c, treeURL, s.headers("application/vnd.github+json"), &tree); err != nil {
		return nil, false, err
	}
	out := make([]string, 0, len(tree.Tree))
	for _, e := range tree.Tree {
		if e.Type == "blob" {
			out = append(out, e.Path)
		}
	}
	return out, tree.Truncated, nil
}

func (s *githubSource) Read(ctx context.Context, p string, limit int) ([]byte, error) {
	u := s.api + "/repos/" + escapePath(s.repo) + "/contents/" + escapePath(p) + "?ref=" + url.QueryEscape(s.ref)
	return readCapped(ctx, s.c, u, s.headers("application/vnd.github.raw+json"), limit)
}

type gitlabSource struct {
	c                     *http.Client
	api, repo, ref, token string
}

func (s *gitlabSource) headers() map[string]string {
	h := map[string]string{}
	if s.token != "" {
		h["Authorization"] = "Bearer " + s.token
	}
	return h
}

func (s *gitlabSource) Paths(ctx context.Context) ([]string, bool, error) {
	var out []string
	proj := url.PathEscape(s.repo)
	for page := 1; page <= maxListPages; page++ {
		var entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		}
		u := fmt.Sprintf("%s/projects/%s/repository/tree?recursive=true&per_page=100&page=%d&ref=%s", s.api, proj, page, url.QueryEscape(s.ref))
		if err := getJSON(ctx, s.c, u, s.headers(), &entries); err != nil {
			return nil, false, err
		}
		for _, e := range entries {
			if e.Type == "blob" {
				out = append(out, e.Path)
			}
		}
		if len(entries) < 100 {
			return out, false, nil
		}
	}
	return out, true, nil
}

func (s *gitlabSource) Read(ctx context.Context, p string, limit int) ([]byte, error) {
	u := s.api + "/projects/" + url.PathEscape(s.repo) + "/repository/files/" + url.PathEscape(p) + "/raw?ref=" + url.QueryEscape(s.ref)
	return readCapped(ctx, s.c, u, s.headers(), limit)
}

type giteaSource struct {
	c                     *http.Client
	api, repo, ref, token string
}

func (s *giteaSource) headers() map[string]string {
	h := map[string]string{}
	if s.token != "" {
		h["Authorization"] = "token " + s.token
	}
	return h
}

func (s *giteaSource) Paths(ctx context.Context) ([]string, bool, error) {
	var out []string
	for page := 1; page <= maxListPages; page++ {
		var tree struct {
			Truncated bool `json:"truncated"`
			Tree      []struct {
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"tree"`
		}
		u := s.api + "/repos/" + escapePath(s.repo) + "/git/trees/" + url.PathEscape(s.ref) + "?recursive=true&per_page=1000&page=" + strconv.Itoa(page)
		if err := getJSON(ctx, s.c, u, s.headers(), &tree); err != nil {
			return nil, false, err
		}
		for _, e := range tree.Tree {
			if e.Type == "blob" {
				out = append(out, e.Path)
			}
		}
		if len(tree.Tree) < 1000 {
			return out, false, nil
		}
	}
	return out, true, nil
}

func (s *giteaSource) Read(ctx context.Context, p string, limit int) ([]byte, error) {
	u := s.api + "/repos/" + escapePath(s.repo) + "/raw/" + escapePath(p) + "?ref=" + url.QueryEscape(s.ref)
	return readCapped(ctx, s.c, u, s.headers(), limit)
}

func readCapped(ctx context.Context, c *http.Client, u string, hdr map[string]string, limit int) ([]byte, error) {
	b, err := doGet(ctx, c, u, hdr, int64(limit))
	if err != nil {
		return nil, err
	}
	return b, nil
}
