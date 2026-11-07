package webhook

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type desiredAttempts struct {
	fakeAttemptStore
	cache string
}

func (d *desiredAttempts) GetDesiredService(_ context.Context, _ string) (*store.DesiredService, error) {
	return &store.DesiredService{NextCache: d.cache}, nil
}

func TestPushDeployUsesStoredNextCache(t *testing.T) {
	tests := []struct {
		name, stored, specCache, want string
	}{
		{"stored redis wins", spec.NextCacheRedis, "", spec.NextCacheRedis},
		{"stored none overrides spec redis", spec.NextCacheNone, spec.NextCacheRedis, spec.NextCacheNone},
		{"empty stored keeps spec", "", spec.NextCacheRedis, spec.NextCacheRedis},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig()
			if tt.specCache != "" {
				cfg.Service.Next = &spec.Next{Cache: tt.specCache}
			}
			deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
			h := New(cfg, deployer, &desiredAttempts{cache: tt.stored}, nil, nil, nil, discardLogger())
			h.fetch = func(_ context.Context, _, _ string, _ func(build.ProgressEvent)) (string, func(), error) {
				return "/tmp/checkout-dir", func() {}, nil
			}
			body := pushBody(t, "refs/heads/main", "sha1")
			rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
			}
			got := ""
			if n := deployer.lastReq.Service.Next; n != nil {
				got = n.Cache
			}
			if got != tt.want {
				t.Errorf("next cache = %q, want %q", got, tt.want)
			}
		})
	}
}
