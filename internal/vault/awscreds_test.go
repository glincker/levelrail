package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeAWSVault struct {
	mu      sync.Mutex
	reads   int
	revoked []string
	ttlSeen string
}

func (f *fakeAWSVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Vault-Token") != "tok" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/aws/creds/lr":
		f.reads++
		f.ttlSeen = r.URL.Query().Get("ttl")
		_ = json.NewEncoder(w).Encode(map[string]any{"lease_id": fmt.Sprintf("aws/creds/lr/%d", f.reads), "lease_duration": 1000,
			"data": map[string]any{"access_key": fmt.Sprintf("ASIA%d", f.reads), "secret_key": "s", "security_token": "t"}})
	case r.URL.Path == "/v1/sys/leases/revoke":
		var body struct {
			LeaseID string `json:"lease_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.revoked = append(f.revoked, body.LeaseID)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestAWSBrokerCachesRenewsLazilyAndRevokes(t *testing.T) {
	fv := &fakeAWSVault{}
	srv := httptest.NewServer(fv)
	defer srv.Close()
	b := NewAWSBroker(func(context.Context) Tunables { return Tunables{Timeout: 5 * time.Second, AllowPrivate: true} })
	now := time.Now()
	b.now = func() time.Time { return now }
	spec := AWSSpec{Account: "acct", Address: srv.URL, Role: "lr"}
	ctx := context.Background()

	first, err := b.Credentials(ctx, spec, "tok")
	if err != nil || first.AccessKeyID != "ASIA1" || fv.ttlSeen != "900s" {
		t.Fatalf("first = %+v, ttl %q, err %v", first, fv.ttlSeen, err)
	}
	now = now.Add(700 * time.Second)
	if again, _ := b.Credentials(ctx, spec, "tok"); again.AccessKeyID != "ASIA1" || fv.reads != 1 {
		t.Fatalf("cache miss before 80 percent: reads %d", fv.reads)
	}
	now = now.Add(200 * time.Second)
	renewed, err := b.Credentials(ctx, spec, "tok")
	if err != nil || renewed.AccessKeyID != "ASIA2" || fv.reads != 2 {
		t.Fatalf("renew = %+v reads %d err %v", renewed, fv.reads, err)
	}
	if len(fv.revoked) != 1 || fv.revoked[0] != "aws/creds/lr/1" {
		t.Fatalf("old lease not revoked: %v", fv.revoked)
	}
	if err := b.Revoke(ctx, "acct", "tok"); err != nil || len(fv.revoked) != 2 || fv.revoked[1] != "aws/creds/lr/2" {
		t.Fatalf("revoke on delete: %v %v", fv.revoked, err)
	}
	if _, err := b.Credentials(ctx, spec, "tok"); err != nil || fv.reads != 3 {
		t.Fatalf("cache not cleared after revoke: reads %d", fv.reads)
	}
}

func TestAWSBrokerRejectsBadToken(t *testing.T) {
	srv := httptest.NewServer(&fakeAWSVault{})
	defer srv.Close()
	b := NewAWSBroker(func(context.Context) Tunables { return Tunables{Timeout: 5 * time.Second, AllowPrivate: true} })
	if _, err := b.Credentials(context.Background(), AWSSpec{Account: "a", Address: srv.URL, Role: "lr"}, "wrong"); err == nil {
		t.Fatal("bad token accepted")
	}
}

func TestAWSBrokerBlocksPrivateAddressByDefault(t *testing.T) {
	srv := httptest.NewServer(&fakeAWSVault{})
	defer srv.Close()
	b := NewAWSBroker(func(context.Context) Tunables { return Tunables{Timeout: 5 * time.Second} })
	if _, err := b.Credentials(context.Background(), AWSSpec{Account: "a", Address: srv.URL, Role: "lr"}, "tok"); err == nil {
		t.Fatal("loopback vault accepted without allow_private")
	}
}
