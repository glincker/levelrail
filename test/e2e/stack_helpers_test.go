package e2e

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

// bearerTransport injects a bearer token so a token-authenticated client
// can share requestJSON with the cookie-session clients.
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// memSecrets is an in-memory api.SecretSetter. Only the methods the stack
// apply path touches are implemented; the embedded nil interface panics on
// any other call, which would flag an unexpected new dependency.
type memSecrets struct {
	api.SecretSetter
	values map[string]string
}

func (m *memSecrets) SetValueGuarded(_ context.Context, svc, key, plain string, _ bool) error {
	m.values[svc+"/"+key] = plain
	return nil
}

func (m *memSecrets) Exists(_ context.Context, svc, key string) (bool, error) {
	_, ok := m.values[svc+"/"+key]
	return ok, nil
}

func (m *memSecrets) ListKeys(context.Context, string) ([]store.SecretKeyInfo, error) {
	return nil, nil
}

func (m *memSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	return m.values[svc+"/"+key], nil
}

func (m *memSecrets) ExistsForServices(_ context.Context, svcs []string, key string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, s := range svcs {
		_, out[s] = m.values[s+"/"+key]
	}
	return out, nil
}
