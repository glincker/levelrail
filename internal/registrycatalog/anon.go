package registrycatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	anonPageSize   = 1000
	anonMaxPages   = 20
	anonMaxBody    = 4 << 20
	anonMaxRetryIn = time.Hour
)

// RateLimitError reports an upstream 429 together with its Retry-After.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("registrycatalog: rate limited, retry after %s", e.RetryAfter)
}

type bearerToken struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

// ListTagsAnonymous lists every tag of a public repository, following the
// anonymous bearer token flow and Link pagination of the OCI distribution API.
func (c *Client) ListTagsAnonymous(ctx context.Context, baseURL, repository string) ([]string, error) {
	next := baseURL + "/v2/" + escapeRepositoryPath(repository) + "/tags/list?n=" + strconv.Itoa(anonPageSize)
	var tags []string
	token := ""
	for page := 0; page < anonMaxPages && next != ""; page++ {
		body, link, err := c.anonGet(ctx, next, &token, repository)
		if err != nil {
			return nil, fmt.Errorf("registrycatalog: list tags for %q: %w", repository, err)
		}
		var out tagsListResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("registrycatalog: decode tags for %q: %w", repository, err)
		}
		tags = append(tags, out.Tags...)
		next = nextLink(baseURL, link)
	}
	return tags, nil
}

func nextLink(baseURL, header string) string {
	end := strings.Index(header, ">")
	if !strings.HasPrefix(header, "<") || end < 0 || !strings.Contains(header, `rel="next"`) {
		return ""
	}
	ref := header[1:end]
	if strings.HasPrefix(ref, "/") {
		return baseURL + ref
	}
	if strings.HasPrefix(ref, baseURL+"/") {
		return ref
	}
	return ""
}

func (c *Client) anonGet(ctx context.Context, target string, token *string, repository string) ([]byte, string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, "", fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if *token != "" {
			req.Header.Set("Authorization", "Bearer "+*token)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("request %s: %w", target, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, anonMaxBody))
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			t, err := c.fetchToken(ctx, resp.Header.Get("WWW-Authenticate"), repository)
			if err != nil {
				return nil, "", err
			}
			*token = t
			continue
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, "", &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
		case resp.StatusCode == http.StatusNotFound:
			return nil, "", ErrNotFound
		case resp.StatusCode < 200 || resp.StatusCode >= 300:
			return nil, "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, target)
		}
		if readErr != nil {
			return nil, "", fmt.Errorf("read %s: %w", target, readErr)
		}
		return body, resp.Header.Get("Link"), nil
	}
	return nil, "", errors.New("registry refused the anonymous token")
}

func (c *Client) fetchToken(ctx context.Context, challenge, repository string) (string, error) {
	params := parseChallenge(challenge)
	u, err := url.Parse(params["realm"])
	if err != nil || u.Scheme != "https" {
		return "", errors.New("registry sent no usable https token realm")
	}
	q := u.Query()
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	q.Set("scope", "repository:"+repository+":pull")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}
	var bt bearerToken
	if err := json.NewDecoder(io.LimitReader(resp.Body, anonMaxBody)).Decode(&bt); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if bt.Token != "" {
		return bt.Token, nil
	}
	if bt.AccessToken != "" {
		return bt.AccessToken, nil
	}
	return "", errors.New("token endpoint returned no token")
}

func parseChallenge(h string) map[string]string {
	out := map[string]string{}
	h = strings.TrimSpace(strings.TrimPrefix(h, "Bearer"))
	for _, part := range strings.Split(h, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			out[k] = strings.Trim(v, `"`)
		}
	}
	return out
}

func parseRetryAfter(h string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && n > 0 {
		d := time.Duration(n) * time.Second
		if d > anonMaxRetryIn {
			return anonMaxRetryIn
		}
		return d
	}
	return time.Minute
}
