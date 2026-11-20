package build

import (
	"context"
	"testing"
	"time"
)

func TestCacheForTier_PersonalGetsNoCachePush(t *testing.T) {
	r := &Router{nodes: func(context.Context) ([]NodeInfo, error) {
		return []NodeInfo{{ID: "p", Trust: TrustPersonal}, {ID: "s", Trust: TrustServer}}, nil
	}}
	req := RemoteRequest{Cache: CacheConfig{RegistryRef: "reg.example/buildcache/app:c"}}
	pr, repo := r.cacheForTier(context.Background(), "p", req, "reg.example")
	if repo != "" || pr.Cache.RegistryRef != "" {
		t.Fatalf("personal tier kept cache: %q %q", repo, pr.Cache.RegistryRef)
	}
	iss, err := NewPushTokenIssuer(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := iss.IssueWithCache("reg.example/levelrail/app", "b1", repo, time.Hour)
	if _, _, err := iss.verifyScope(tok, "reg.example/buildcache/app"); err == nil {
		t.Fatal("personal token must not address the cache repo")
	}
	sr, srepo := r.cacheForTier(context.Background(), "s", req, "reg.example")
	if srepo == "" || sr.Cache.RegistryRef == "" {
		t.Fatal("server tier must keep the cache")
	}
}
