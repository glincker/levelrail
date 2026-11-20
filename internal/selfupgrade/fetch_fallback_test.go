package selfupgrade

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func releaseServer(t *testing.T, signed bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+bundleAsset):
			if !signed {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte("{}"))
		default:
			_, _ = w.Write([]byte("data"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchFallsBackOnlyToASignedMirror(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	t.Cleanup(down.Close)
	tests := []struct {
		name     string
		primary  string
		fallback *httptest.Server
		wantErr  error
		wantSig  bool
	}{
		{"primary works, fallback unused", releaseServer(t, true).URL, nil, nil, true},
		{"primary down, signed mirror answers", down.URL, releaseServer(t, true), nil, true},
		{"primary down, unsigned mirror refused", down.URL, releaseServer(t, false), ErrFallbackUnsigned, false},
		{"primary down, no fallback", down.URL, nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Fetcher{BaseURL: tt.primary, AllowHTTP: true}
			if tt.fallback != nil {
				f.FallbackURL = tt.fallback.URL
			}
			got, err := f.Fetch(t.Context(), "v1.0.0", "levelrail-linux-amd64", t.TempDir())
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.name == "primary down, no fallback":
				if err == nil {
					t.Fatal("expected the primary failure to surface")
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			case (got.BundlePath != "") != tt.wantSig:
				t.Fatalf("bundle path = %q", got.BundlePath)
			}
		})
	}
}
