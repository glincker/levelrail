package proxyroutes

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRenderRedirectSkipsSchemeHop(t *testing.T) {
	traefik := Route{Domain: "www.example.com", EntrypointHTTP: "http", EntrypointHTTPS: "https", Upstream: "127.0.0.1:8088"}
	ng := nginxRoute()
	tests := []struct {
		name     string
		render   func(redirect bool) (string, error)
		redirect bool
		has      []string
		lacks    []string
	}{
		{name: "traefik plain", redirect: false, has: []string{"redirectScheme", "middlewares: ["}, render: func(b bool) (string, error) {
			r := traefik
			r.Redirect = b
			out, err := testNS.Render(r)
			return string(out), err
		}},
		{name: "traefik redirected", redirect: true, has: []string{"-http:", "services:"}, lacks: []string{"redirectScheme", "middlewares"}, render: func(b bool) (string, error) {
			r := traefik
			r.Redirect = b
			out, err := testNS.Render(r)
			return string(out), err
		}},
		{name: "nginx plain", redirect: false, has: []string{"return 301 https://$host$request_uri"}, render: func(b bool) (string, error) {
			r := ng
			r.Redirect = b
			out, err := nginxProvider{}.Render(testNS, r)
			return string(out), err
		}},
		{name: "nginx redirected", redirect: true, has: []string{"proxy_pass http://host.docker.internal:8088;"}, lacks: []string{"return 301 https://$host"}, render: func(b bool) (string, error) {
			r := ng
			r.Redirect = b
			out, err := nginxProvider{}.Render(testNS, r)
			return string(out), err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := tc.render(tc.redirect)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range tc.has {
				if !strings.Contains(s, h) {
					t.Errorf("missing %q in:\n%s", h, s)
				}
			}
			for _, l := range tc.lacks {
				if strings.Contains(s, l) {
					t.Errorf("unexpected %q in:\n%s", l, s)
				}
			}
		})
	}
}

type redirectSyncStore struct {
	*fakeSyncStore
	redirects []store.DomainRedirect
}

func (f *redirectSyncStore) ListDomainRedirects(context.Context) ([]store.DomainRedirect, error) {
	return f.redirects, nil
}

func TestPlanFollowsRedirectSetAndClear(t *testing.T) {
	st := &redirectSyncStore{fakeSyncStore: newFakeSyncStore(t.TempDir())}
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"www.example.com", "example.com"}}}
	s := newTestSyncer(st)
	ctx := context.Background()
	hasScheme := func() map[string]bool {
		p, err := s.Plan(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, it := range p.Items {
			out[it.Domain] = strings.Contains(string(it.Content), "redirectScheme")
		}
		return out
	}
	if got := hasScheme(); !got["www.example.com"] || !got["example.com"] {
		t.Fatalf("no redirect: both keep the scheme hop, got %v", got)
	}
	st.redirects = []store.DomainRedirect{{Domain: "www.example.com", TargetURL: "https://example.com", StatusCode: 301}}
	if got := hasScheme(); got["www.example.com"] || !got["example.com"] {
		t.Fatalf("redirect set: only www drops the hop, got %v", got)
	}
	st.redirects = nil
	if got := hasScheme(); !got["www.example.com"] {
		t.Fatalf("redirect cleared: hop returns, got %v", got)
	}
}
