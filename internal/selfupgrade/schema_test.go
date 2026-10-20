package selfupgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

type schemaRelease struct {
	manifest string
	sumOf    string
	bundle   bool
}

func serveSchemaRelease(t *testing.T, r schemaRelease) Fetcher {
	t.Helper()
	sumOf := r.sumOf
	if sumOf == "" {
		sumOf = r.manifest
	}
	digest := sha256.Sum256([]byte(sumOf))
	mux := http.NewServeMux()
	if r.manifest != "" {
		mux.HandleFunc("/dl/v1.2.3/"+upgrade.ManifestAsset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(r.manifest)) })
	}
	mux.HandleFunc("/dl/v1.2.3/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(hex.EncodeToString(digest[:]) + "  " + upgrade.ManifestAsset + "\n"))
	})
	if r.bundle {
		mux.HandleFunc("/dl/v1.2.3/"+bundleAsset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) })
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return Fetcher{BaseURL: srv.URL + "/dl", AllowHTTP: true}
}

func okSigner() Signer {
	return Signer{Mode: VerifyAuto, Run: func(context.Context, string, ...string) ([]byte, error) { return nil, nil }}
}

func TestFetchReleaseSchema(t *testing.T) {
	good := `{"version":"v1.2.3","schema_version":1010,"os":"linux","arch":"amd64"}`
	tests := []struct {
		name    string
		rel     schemaRelease
		signer  Signer
		want    int
		wantErr error
		wantMsg string
	}{
		{name: "signed manifest", rel: schemaRelease{manifest: good, bundle: true}, signer: okSigner(), want: 1010},
		{name: "unsigned release in auto mode is checksum only", rel: schemaRelease{manifest: good}, signer: Signer{Mode: VerifyAuto, Run: okSigner().Run}, want: 1010},
		{name: "unsigned release on a pinned host", rel: schemaRelease{manifest: good}, signer: Signer{Mode: VerifyAuto, Pinned: true, Run: okSigner().Run}, wantErr: ErrSignaturePinned},
		{name: "invalid signature", rel: schemaRelease{manifest: good, bundle: true},
			signer: Signer{Mode: VerifyAuto, Run: func(context.Context, string, ...string) ([]byte, error) {
				return []byte("no match"), errors.New("exit 1")
			}}, wantErr: ErrSignatureInvalid},
		{name: "manifest not in the signed checksums", rel: schemaRelease{manifest: good, sumOf: "something else", bundle: true}, signer: okSigner(), wantErr: ErrChecksumMismatch},
		{name: "malformed manifest", rel: schemaRelease{manifest: "not json", bundle: true}, signer: okSigner(), wantErr: ErrManifestInvalid},
		{name: "manifest for another version", rel: schemaRelease{manifest: `{"version":"v0.1.0","schema_version":5}`, bundle: true}, signer: okSigner(), wantErr: ErrManifestInvalid},
		{name: "zero schema", rel: schemaRelease{manifest: `{"version":"v1.2.3","schema_version":0}`, bundle: true}, signer: okSigner(), wantErr: ErrManifestInvalid},
		{name: "oversized manifest", rel: schemaRelease{manifest: `{"version":"v1.2.3","schema_version":5,"pad":"` + strings.Repeat("a", manifestMaxBytes) + `"}`, bundle: true}, signer: okSigner(), wantMsg: "exceeds"},
		{name: "release without a manifest", rel: schemaRelease{bundle: true}, signer: okSigner(), wantMsg: "404"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FetchReleaseSchema(context.Background(), serveSchemaRelease(t, tc.rel), tc.signer, "v1.2.3", t.TempDir())
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			case tc.wantMsg != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
					t.Fatalf("err = %v, want text %q", err, tc.wantMsg)
				}
			case err != nil:
				t.Fatal(err)
			case got.Schema != tc.want:
				t.Fatalf("schema = %d, want %d", got.Schema, tc.want)
			}
		})
	}
}

func TestFetchReleaseSchemaRefusesUnsafeTagAndRedirect(t *testing.T) {
	f := serveSchemaRelease(t, schemaRelease{manifest: `{"version":"v1.2.3","schema_version":1}`})
	if _, err := FetchReleaseSchema(context.Background(), f, okSigner(), "../etc", t.TempDir()); err == nil {
		t.Fatal("unsafe tag accepted")
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("internal")) }))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/secret", http.StatusFound)
	}))
	defer redirector.Close()
	rf := Fetcher{BaseURL: redirector.URL + "/dl", AllowHTTP: true, RedirectHosts: []string{"github.com", "githubusercontent.com"}}
	_, err := FetchReleaseSchema(context.Background(), rf, okSigner(), "v1.2.3", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "host not allowed") {
		t.Fatalf("redirect off the allowlist was followed: %v", err)
	}
}

func TestDowngradeMessageMatchesGuard(t *testing.T) {
	want := upgrade.DowngradeMessage(2160, "v0.6.0", 1010)
	if !strings.HasPrefix(ErrDowngrade.Error(), upgrade.DowngradeRefusal) || !strings.HasPrefix(want, ErrDowngrade.Error()+": database is at schema 2160, v0.6.0 supports 1010") {
		t.Fatalf("check text %q drifted from the guard %q", want, ErrDowngrade)
	}
}
