package egress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func samplePolicy(version int64, secret, host string) Policy {
	return Policy{Version: version, Plugins: []PluginPolicy{
		{Name: "alpha", SecretHash: HexHash(HashSecret(secret)), Hosts: []string{host, "api.example.net:8443"}},
	}}
}

func TestPolicyRejectsInvalidInput(t *testing.T) {
	good := HexHash(HashSecret("x"))
	cases := map[string]Policy{
		"empty name":    {Plugins: []PluginPolicy{{Name: "", SecretHash: good}}},
		"colon name":    {Plugins: []PluginPolicy{{Name: "a:b", SecretHash: good}}},
		"duplicate":     {Plugins: []PluginPolicy{{Name: "a", SecretHash: good}, {Name: "a", SecretHash: good}}},
		"bad hash":      {Plugins: []PluginPolicy{{Name: "a", SecretHash: "zz"}}},
		"empty host":    {Plugins: []PluginPolicy{{Name: "a", SecretHash: good, Hosts: []string{""}}}},
		"wildcard host": {Plugins: []PluginPolicy{{Name: "a", SecretHash: good, Hosts: []string{"*.example.com"}}}},
		"bad port":      {Plugins: []PluginPolicy{{Name: "a", SecretHash: good, Hosts: []string{"h.example:99999"}}}},
		"path in host":  {Plugins: []PluginPolicy{{Name: "a", SecretHash: good, Hosts: []string{"h.example/x"}}}},
		"negative ver":  {Version: -1},
	}
	for name, p := range cases {
		if err := p.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
		if err := NewPolicyStore().Set(p); err == nil {
			t.Errorf("%s: store accepted it", name)
		}
	}
	if _, err := DecodePolicy(strings.NewReader(`{"version":1,"plugins":[],"extra":1}`)); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := DecodePolicy(strings.NewReader(`{"version":1,"plugins":[]} {}`)); err == nil {
		t.Error("trailing data accepted")
	}
}

func TestPolicyStoreDeniesWithoutPolicy(t *testing.T) {
	st := NewPolicyStore()
	if st.Ready() || st.Verify("alpha", "s") || st.Allowed("alpha", "h.example", 443) {
		t.Fatal("empty store must deny everything")
	}
	srv := httptest.NewServer(NewPolicyServer(st, "").Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("healthz status %d", resp.StatusCode)
	}
}

func TestPolicyStoreSwapUnderLoad(t *testing.T) {
	st := NewPolicyStore()
	if err := st.Set(samplePolicy(1, "s1", "h1.example")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if st.Verify("alpha", "other") {
					t.Error("wrong secret accepted")
					return
				}
				if !st.Verify("alpha", "s1") {
					t.Error("credential missing during swap")
					return
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		h := "h1.example"
		if i%2 == 1 {
			h = "h2.example"
		}
		if err := st.Set(samplePolicy(int64(i+2), "s1", h)); err != nil {
			t.Error(err)
		}
	}
	close(stop)
	wg.Wait()
	if !st.Allowed("alpha", "api.example.net", 8443) {
		t.Error("host:port entry not allowed")
	}
}

func TestPolicyChannelRoundTripAndSocketMode(t *testing.T) {
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "run", "policy.sock")
	cache := filepath.Join(dir, "cache", "policy.json")
	st := NewPolicyStore()
	ps := NewPolicyServer(st, cache)
	ln, err := ListenPolicySocket(sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: ps.Handler(), ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	fi, err := os.Stat(sock)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v err %v", fi, err)
	}
	di, err := os.Stat(filepath.Dir(sock))
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("socket dir mode %v err %v", di, err)
	}

	if err := PushPolicy(context.Background(), sock, Policy{Plugins: []PluginPolicy{{Name: "a:b"}}}); err == nil {
		t.Fatal("invalid policy pushed")
	}
	if st.Ready() {
		t.Fatal("invalid push changed state")
	}
	if err := PushPolicy(context.Background(), sock, samplePolicy(7, "tok", "h.example")); err != nil {
		t.Fatal(err)
	}
	if !st.Ready() || st.Version() != 7 || !st.Verify("alpha", "tok") || !st.Allowed("alpha", "h.example", 443) {
		t.Fatal("pushed policy not applied")
	}

	cfi, err := os.Stat(cache)
	if err != nil || cfi.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode %v err %v", cfi, err)
	}
	b, _ := os.ReadFile(filepath.Clean(cache))
	if strings.Contains(string(b), "tok") {
		t.Fatal("cache holds a plain secret")
	}

	// Restart: a fresh store reloads the cache without a push.
	st2 := NewPolicyStore()
	if err := NewPolicyServer(st2, cache).LoadCache(); err != nil {
		t.Fatal(err)
	}
	if !st2.Verify("alpha", "tok") || st2.Version() != 7 {
		t.Fatal("cache not reloaded")
	}

	// A corrupt cache fails closed.
	if err := os.WriteFile(cache, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	st3 := NewPolicyStore()
	if err := NewPolicyServer(st3, cache).LoadCache(); err == nil || st3.Ready() {
		t.Fatal("corrupt cache must not install a policy")
	}
}

// Unix socket paths are length limited on some systems, so avoid long temp roots.
func shortTempDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "egr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}
