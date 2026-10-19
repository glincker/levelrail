package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func identityServer(t *testing.T, info string, tokenHits *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == instancePath {
			if info == "" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(info))
			return
		}
		tokenHits.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
}

const serverAInfo = `{"instance_id":"inst_AAAAAAAAAAAAAAAA","name":"Alpha","fingerprint":"aaaa-bbbb"}`
const serverBInfo = `{"instance_id":"inst_BBBBBBBBBBBBBBBB","name":"Beta","fingerprint":"cccc-dddd"}`

func TestPinnedToken_RefusedOnWrongServer(t *testing.T) {
	ResetServerPins()
	var hits atomic.Int32
	b := identityServer(t, serverBInfo, &hits)
	defer b.Close()

	registerTokenPin("tok-a", tokenPin{
		pin:     ServerPin{InstanceID: "inst_AAAAAAAAAAAAAAAA", Fingerprint: "aaaa-bbbb", Name: "Alpha"},
		enforce: true, prog: "cli", profile: "default",
	})
	err := NewClient(b.URL, "tok-a").do(context.Background(), http.MethodGet, "/api/v1/apps", nil, nil)
	var mismatch *ServerMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("err = %v, want ServerMismatchError", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("token reached the wrong server %d times", hits.Load())
	}
	if !strings.Contains(err.Error(), "auth login --reauth") || !strings.Contains(err.Error(), "auth trust-server") {
		t.Errorf("error lacks the fix commands: %v", err)
	}
}

func TestPinnedToken_AllowedOnMatchingServer(t *testing.T) {
	ResetServerPins()
	var hits atomic.Int32
	a := identityServer(t, serverAInfo, &hits)
	defer a.Close()
	registerTokenPin("tok-a", tokenPin{
		pin:     ServerPin{InstanceID: "inst_AAAAAAAAAAAAAAAA", Fingerprint: "aaaa-bbbb"},
		enforce: true, prog: "cli", profile: "default",
	})
	if err := NewClient(a.URL, "tok-a").do(context.Background(), http.MethodGet, "/api/v1/apps", nil, nil); err != nil {
		t.Fatalf("matching server: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", hits.Load())
	}
}

func TestPinnedToken_ServerWithoutIdentityRefused(t *testing.T) {
	ResetServerPins()
	var hits atomic.Int32
	old := identityServer(t, "", &hits)
	defer old.Close()
	registerTokenPin("tok-a", tokenPin{pin: ServerPin{InstanceID: "inst_AAAAAAAAAAAAAAAA", Fingerprint: "aaaa-bbbb"}, enforce: true, prog: "cli", profile: "p"})
	err := NewClient(old.URL, "tok-a").do(context.Background(), http.MethodGet, "/api/v1/apps", nil, nil)
	var mismatch *ServerMismatchError
	if !errors.As(err, &mismatch) || hits.Load() != 0 {
		t.Fatalf("err = %v hits = %d, want refusal with nothing sent", err, hits.Load())
	}
}

func TestEnvToken_WarnsButSends(t *testing.T) {
	ResetServerPins()
	var warn strings.Builder
	SetWarnWriter(&warn)
	defer SetWarnWriter(nil)
	var hits atomic.Int32
	b := identityServer(t, serverBInfo, &hits)
	defer b.Close()
	registerTokenPin("env-tok", tokenPin{pin: ServerPin{InstanceID: "inst_AAAAAAAAAAAAAAAA", Fingerprint: "aaaa-bbbb"}, prog: "cli", profile: "default"})
	if err := NewClient(b.URL, "env-tok").do(context.Background(), http.MethodGet, "/api/v1/apps", nil, nil); err != nil {
		t.Fatalf("env token must keep working: %v", err)
	}
	if hits.Load() != 1 || !strings.Contains(warn.String(), "warning") {
		t.Fatalf("hits = %d warn = %q, want sent with a warning", hits.Load(), warn.String())
	}
}

func TestUnpinnedProfile_TrustOnFirstUse(t *testing.T) {
	ResetServerPins()
	defer SetTrustHook(nil)
	var hits atomic.Int32
	a := identityServer(t, serverAInfo, &hits)
	defer a.Close()

	SetTrustHook(func(TrustRequest) error { return errors.New("declined") })
	registerTokenPin("tok-new", tokenPin{enforce: true, prog: "cli", profile: "default"})
	if err := NewClient(a.URL, "tok-new").do(context.Background(), http.MethodGet, "/api/v1/apps", nil, nil); err == nil || hits.Load() != 0 {
		t.Fatalf("declined trust: err = %v hits = %d, want refusal with nothing sent", err, hits.Load())
	}

	ResetServerPins()
	var seen TrustRequest
	SetTrustHook(func(r TrustRequest) error { seen = r; return nil })
	registerTokenPin("tok-new", tokenPin{enforce: true, prog: "cli", profile: "default"})
	if err := NewClient(a.URL, "tok-new").do(context.Background(), http.MethodGet, "/api/v1/apps", nil, nil); err != nil {
		t.Fatalf("accepted trust: %v", err)
	}
	if seen.Info.InstanceID != "inst_AAAAAAAAAAAAAAAA" || hits.Load() != 1 {
		t.Fatalf("seen = %+v hits = %d", seen, hits.Load())
	}
}

func TestCredentialsFile_RoundTripsServerPin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	in := Credentials{APIURL: "https://x", Token: "t", InstanceID: "inst_1", Fingerprint: "ab-cd", ServerName: "Alpha"}
	if err := WriteCredentialsFile("cli", "default", in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadCredentialsFile("cli", "default")
	if err != nil || out != in {
		t.Fatalf("round trip = %+v, %v, want %+v", out, err, in)
	}
}
