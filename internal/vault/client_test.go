package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type counters struct{ logins, reads atomic.Int32 }

// countingServer issues AppRole tokens with the given lease and counts logins and reads.
func countingServer(t *testing.T, loginStatus int) (*httptest.Server, *counters) {
	t.Helper()
	c := &counters{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/approle/login", func(w http.ResponseWriter, _ *http.Request) {
		c.logins.Add(1)
		time.Sleep(20 * time.Millisecond) // widen the window so concurrent callers overlap
		if loginStatus != 0 {
			w.WriteHeader(loginStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"client_token": "tok", "lease_duration": 100}})
	})
	mux.HandleFunc("/v1/secret/data/app", func(w http.ResponseWriter, r *http.Request) {
		c.reads.Add(1)
		if r.Header.Get("X-Vault-Token") != "tok" && r.Header.Get("X-Vault-Token") != "static" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": map[string]any{"k": "v"}, "metadata": map[string]any{}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, c
}

func clocked(ttl time.Duration) (*Resolver, *clock) {
	r := NewResolverWith(func(context.Context) Tunables {
		return Tunables{Timeout: 5 * time.Second, CacheTTL: ttl, AllowPrivate: true, RenewPercent: 80}
	})
	ck := &clock{t: time.Unix(1_000_000, 0)}
	r.now = ck.now
	return r, ck
}

func appRoleCfg(addr string) store.VaultSettings {
	return store.VaultSettings{Address: addr, AuthMethod: store.VaultAuthMethodAppRole, RoleID: "r", MountPath: "secret"}
}

func TestVaultAddressGuardBlocksLinkLocalAndPrivate(t *testing.T) {
	deny := NewResolverWith(func(context.Context) Tunables { return Tunables{Timeout: time.Second} })
	srv, _ := countingServer(t, 0)
	cfg := store.VaultSettings{Address: srv.URL, AuthMethod: store.VaultAuthMethodToken}
	if _, err := deny.Resolve(context.Background(), cfg, "static", "app", "k"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("loopback with private addresses off must be blocked, got %v", err)
	}
	allow := NewResolverWith(func(context.Context) Tunables { return Tunables{Timeout: time.Second, AllowPrivate: true} })
	cfg.Address = "http://169.254.169.254"
	if _, err := allow.Resolve(context.Background(), cfg, "static", "app", "k"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("metadata address must stay blocked even when private is allowed, got %v", err)
	}
}

func TestVaultRedirectToOtherHostBlocked(t *testing.T) {
	other, _ := countingServer(t, 0)
	redir := httptest.NewServer(http.RedirectHandler(other.URL+"/v1/secret/data/app", http.StatusTemporaryRedirect))
	t.Cleanup(redir.Close)
	r, _ := clocked(0)
	cfg := store.VaultSettings{Address: redir.URL, AuthMethod: store.VaultAuthMethodToken}
	if _, err := r.Resolve(context.Background(), cfg, "static", "app", "k"); !errors.Is(err, ErrRedirectBlocked) {
		t.Fatalf("redirect to another host must be blocked, got %v", err)
	}
}

func TestVaultLoginTokenCachedRenewedAndSingleFlight(t *testing.T) {
	srv, c := countingServer(t, 0)
	r, ck := clocked(0)
	cfg := appRoleCfg(srv.URL)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.Resolve(context.Background(), cfg, "sid", "app", "k"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := c.logins.Load(); got != 1 {
		t.Fatalf("concurrent resolves must share one login, got %d", got)
	}
	if _, err := r.Resolve(context.Background(), cfg, "sid", "app", "k"); err != nil || c.logins.Load() != 1 {
		t.Fatalf("a live token must be reused, logins=%d err=%v", c.logins.Load(), err)
	}
	ck.add(81 * time.Second) // past 80 percent of the 100s lease
	if _, err := r.Resolve(context.Background(), cfg, "sid", "app", "k"); err != nil || c.logins.Load() != 2 {
		t.Fatalf("the token must be renewed before expiry, logins=%d err=%v", c.logins.Load(), err)
	}
}

func TestVaultLoginFailureFailsClosed(t *testing.T) {
	srv, c := countingServer(t, http.StatusForbidden)
	r, _ := clocked(60 * time.Second)
	if _, err := r.Resolve(context.Background(), appRoleCfg(srv.URL), "sid", "app", "k"); err == nil {
		t.Fatal("a failed login must fail the read")
	}
	if c.reads.Load() != 0 {
		t.Fatal("no read may happen without a token")
	}
}

func TestVaultReadCacheHitExpiryAndInvalidation(t *testing.T) {
	srv, c := countingServer(t, 0)
	r, ck := clocked(30 * time.Second)
	cfg := appRoleCfg(srv.URL)
	for i := 0; i < 3; i++ {
		if _, err := r.Resolve(context.Background(), cfg, "sid", "app", "k"); err != nil {
			t.Fatal(err)
		}
	}
	if c.reads.Load() != 1 {
		t.Fatalf("reads inside the TTL must hit the cache, got %d", c.reads.Load())
	}
	if _, err := r.Resolve(context.Background(), cfg, "sid2", "app", "k"); err != nil || c.reads.Load() != 2 {
		t.Fatalf("a rotated credential must miss the cache, reads=%d err=%v", c.reads.Load(), err)
	}
	ck.add(31 * time.Second)
	if _, err := r.Resolve(context.Background(), cfg, "sid", "app", "k"); err != nil || c.reads.Load() != 3 {
		t.Fatalf("an expired entry must be read again, reads=%d err=%v", c.reads.Load(), err)
	}
	r.Flush()
	if _, err := r.Resolve(context.Background(), cfg, "sid", "app", "k"); err != nil || c.reads.Load() != 4 {
		t.Fatalf("Flush must drop cached values, reads=%d err=%v", c.reads.Load(), err)
	}
}

func TestVaultPrivateAddressDefaultOnKeepsMetadataBlocked(t *testing.T) {
	// A nil settings service serves registry defaults, so this proves the shipped default.
	tun := TunablesFrom(nil)(context.Background())
	if !tun.AllowPrivate {
		t.Fatal("private addresses must be allowed by default for admin-set connections")
	}
	if !blockedIP(net.ParseIP("169.254.169.254"), tun.AllowPrivate) || blockedIP(net.ParseIP("10.0.0.5"), tun.AllowPrivate) {
		t.Fatal("default must block metadata and allow private ranges")
	}
}
