package imageupdate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/distribution/reference"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// ErrRegistryRequest is the only failure text callers see, so a registry can
// not use error messages to read back internal responses.
var ErrRegistryRequest = errors.New("imageupdate: registry request failed")

// maxRegistryRedirects bounds same-host redirects (blob CDNs are not used for tag lists).
const maxRegistryRedirects = 3

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
	client := guardedClient(l.Client)
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
			return nil, ErrRegistryRequest
		}
		var out struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, ErrRegistryRequest
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
		return nil, "", 0, nil, ErrRegistryRequest
	}
	defer func() { _ = resp.Body.Close() }()
	var raw json.RawMessage
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, "", 0, nil, ErrRegistryRequest
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
	if err != nil || realm.Scheme != "https" || !realmHostAllowed(realm.Hostname()) {
		return "", ErrRegistryRequest
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
		return "", ErrRegistryRequest
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", ErrRegistryRequest
	}
	var out struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", ErrRegistryRequest
	}
	if out.Token != "" {
		return out.Token, nil
	}
	return out.AccessToken, nil
}

// guardedClient wraps c (or a netguard client) so every dial is checked and a
// redirect may only stay on the original host, at most three times.
func guardedClient(c *http.Client) *http.Client {
	if c == nil {
		c = netguard.NewClientPrivateNetworks()
	}
	g := *c
	if g.Timeout == 0 {
		g.Timeout = 30 * time.Second
	}
	g.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRegistryRedirects || req.URL.Host != via[0].URL.Host {
			return ErrRegistryRequest
		}
		return nil
	}
	return &g
}

// realmHostAllowed refuses an IP-literal token realm in any internal range; named hosts are checked again at dial time.
func realmHostAllowed(host string) bool {
	if host == "" {
		return false
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return true
	}
	return !netguard.IsBlocked(addr)
}
