package imageupdate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/distribution/reference"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// TagLister lists every tag of an image repository; ref is any reference of
// that repository (its tag is ignored).
type TagLister interface {
	ListTags(ctx context.Context, ref string, auth *docker.RegistryAuth) ([]string, error)
}

// maxTagPages bounds pagination so a misbehaving registry cannot loop forever.
const maxTagPages = 50

// HTTPTagLister reads /v2/<repo>/tags/list from an OCI registry, following
// Link pagination and the Bearer token challenge.
type HTTPTagLister struct {
	Client *http.Client
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// ListTags implements TagLister.
func (l HTTPTagLister) ListTags(ctx context.Context, ref string, auth *docker.RegistryAuth) ([]string, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, fmt.Errorf("imageupdate: parse %q: %w", ref, err)
	}
	host := reference.Domain(named)
	if host == "docker.io" {
		host = "registry-1.docker.io"
	}
	client := l.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	next := "https://" + host + "/v2/" + reference.Path(named) + "/tags/list"
	var tags []string
	token := ""
	for page := 0; next != "" && page < maxTagPages; page++ {
		body, link, status, hdr, err := l.get(ctx, client, next, token, auth)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized && token == "" {
			if token, err = fetchToken(ctx, client, hdr.Get("Www-Authenticate"), auth); err != nil {
				return nil, err
			}
			page--
			continue
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("imageupdate: list tags of %s: registry answered %d", reference.Path(named), status)
		}
		var out struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("imageupdate: decode tag list: %w", err)
		}
		tags = append(tags, out.Tags...)
		next = ""
		if link != "" {
			base, _ := url.Parse("https://" + host)
			if u, err := base.Parse(link); err == nil && u.Host == host {
				next = u.String()
			}
		}
	}
	return tags, nil
}

func (l HTTPTagLister) get(ctx context.Context, c *http.Client, u, token string, auth *docker.RegistryAuth) (body []byte, link string, status int, hdr http.Header, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", 0, nil, err
	}
	switch {
	case token != "":
		req.Header.Set("Authorization", "Bearer "+token)
	case auth != nil && auth.Bearer != "":
		req.Header.Set("Authorization", "Bearer "+auth.Bearer)
	case auth != nil && auth.Username != "":
		req.SetBasicAuth(auth.Username, auth.Password)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, "", 0, nil, fmt.Errorf("imageupdate: list tags: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var raw json.RawMessage
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, "", 0, nil, fmt.Errorf("imageupdate: read tag list: %w", err)
		}
	}
	if m := nextLink.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
		link = m[1]
	}
	return raw, link, resp.StatusCode, resp.Header, nil
}

// fetchToken answers a Bearer challenge, sending basic credentials when given.
func fetchToken(ctx context.Context, c *http.Client, challenge string, auth *docker.RegistryAuth) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", errors.New("imageupdate: registry requires credentials this app does not have")
	}
	params := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(challenge, -1) {
		params[m[1]] = m[2]
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || realm.Scheme != "https" {
		return "", errors.New("imageupdate: registry sent an unusable token realm")
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
	if auth != nil && auth.Username != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth.Username+":"+auth.Password)))
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("imageupdate: fetch registry token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imageupdate: registry token endpoint answered %d", resp.StatusCode)
	}
	var out struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("imageupdate: decode registry token: %w", err)
	}
	if out.Token != "" {
		return out.Token, nil
	}
	return out.AccessToken, nil
}
