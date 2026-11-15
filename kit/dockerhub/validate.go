package dockerhub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Capability states.
const (
	StatusOK      = "ok"
	StatusDenied  = "denied"
	StatusUnknown = "unknown"
)

// Capability is one checked thing the token can or cannot do.
type Capability struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Validation is what Docker Hub confirmed about a token. Nothing here can
// prove the token is read-only: Docker offers no way to read a personal
// token's permission level and a write is never attempted.
type Validation struct {
	Kind                string       `json:"kind"`
	Username            string       `json:"username"`
	Scopes              []string     `json:"scopes"`
	Capabilities        []Capability `json:"capabilities"`
	PrivateReposVisible bool         `json:"private_repos_visible"`
}

// Validate checks the token against auth.docker.io (a rejected token returns
// ErrUnauthorized), then probes what the Hub API lets it do. Probe failures
// other than a rejection are reported as unknown, not as an error.
func (c *Client) Validate(ctx context.Context, cred Credentials) (Validation, error) {
	if !cred.Present() {
		return Validation{}, ErrUnauthorized
	}
	tok, err := c.registryToken(ctx, cred, "repository:"+previewRepo+":pull")
	if err != nil {
		return Validation{}, err
	}
	v := Validation{Kind: cred.Kind(), Username: cred.Username, Scopes: []string{}, Capabilities: []Capability{}}
	for _, a := range tok.Access {
		v.Scopes = append(v.Scopes, fmt.Sprintf("%s:%s:%s", a.Type, a.Name, strings.Join(a.Actions, ",")))
	}
	v.add("registry_login", StatusOK)

	if _, herr := c.hubBearer(ctx, cred); herr != nil {
		v.add("hub_api", probeStatus(herr))
		return v, nil
	}
	v.add("hub_api", StatusOK)

	page, lerr := c.ListRepositories(ctx, cred, cred.Username, "", 1, 25)
	v.add("repository_list", probeStatus(lerr))
	if lerr != nil || len(page.Repositories) == 0 {
		return v, nil
	}
	pick := page.Repositories[0]
	for _, r := range page.Repositories {
		if r.IsPrivate {
			v.PrivateReposVisible = true
			pick = r
			break
		}
	}
	_, terr := c.ListTagsPage(ctx, cred, pick.Namespace, pick.Name, 1, 1)
	v.add("tag_read", probeStatus(terr))
	ptok, perr := c.registryToken(ctx, cred, "repository:"+pick.FullName+":pull")
	switch {
	case perr != nil:
		v.add("image_pull", probeStatus(perr))
	case hasPull(ptok.Access):
		v.add("image_pull", StatusOK)
	default:
		v.add("image_pull", StatusDenied)
	}
	return v, nil
}

func (v *Validation) add(name, status string) {
	v.Capabilities = append(v.Capabilities, Capability{Name: name, Status: status})
}

func hasPull(access []Access) bool {
	for _, a := range access {
		for _, act := range a.Actions {
			if act == "pull" {
				return true
			}
		}
	}
	return false
}

func probeStatus(err error) string {
	switch {
	case err == nil:
		return StatusOK
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrUnauthorized):
		return StatusDenied
	}
	return StatusUnknown
}

// CachedValidation holds a Validation and when it was read.
type CachedValidation struct {
	Validation Validation
	CheckedAt  time.Time
	Cached     bool
}

// CachedValidate is Validate with an in-memory cache. Failures are not cached.
func (c *Client) CachedValidate(ctx context.Context, cred Credentials, ttl time.Duration, refresh bool) (CachedValidation, error) {
	key := "validate|" + cred.cacheKey()
	if !refresh {
		if v, at, ok := c.cacheLoadAt(key); ok {
			return CachedValidation{Validation: v.(Validation), CheckedAt: at, Cached: true}, nil
		}
	}
	v, err := c.Validate(ctx, cred)
	if err != nil {
		return CachedValidation{}, err
	}
	if ttl > 0 {
		c.cacheStore(key, v, ttl)
	}
	return CachedValidation{Validation: v, CheckedAt: time.Now()}, nil
}
