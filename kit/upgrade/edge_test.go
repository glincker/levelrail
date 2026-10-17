package upgrade

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchLatestEdge_ModelsAnImagePseudoRelease(t *testing.T) {
	const sha = "07cd3bf9a1b2c3d4e5f60718293a4b5c6d7e8f90"
	old := channelHTTPClient
	t.Cleanup(func() { channelHTTPClient = old })
	var gotURL string
	channelHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		body := `{"sha":"` + sha + `","html_url":"https://github.com/Acme/Rail/commit/` + sha + `","commit":{"committer":{"date":"2026-10-01T00:00:00Z"}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}

	got, err := FetchLatestEdge(context.Background(), "Acme/Rail")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotURL, "/repos/Acme/Rail/commits/main") {
		t.Errorf("url = %q", gotURL)
	}
	if got.Tag != "main-07cd3bf" || got.Kind != KindEdgeImage || got.Commit != sha {
		t.Errorf("release = %+v", got)
	}
	if got.Image != "ghcr.io/acme/rail:edge" || !got.IsEdge() || len(got.AssetNames) != 0 {
		t.Errorf("image/edge = %+v", got)
	}
	if !strings.Contains(got.URL, "/commit/") {
		t.Errorf("url should point at the commit: %q", got.URL)
	}
}

func TestUpdateAvailable_EdgeComparesCommitsByPrefix(t *testing.T) {
	full := "main-07cd3bf9a1b2c3d4e5f60718293a4b5c6d7e8f90"
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"built image carries the full sha of the latest commit", full, "main-07cd3bf", false},
		{"older commit", full, "main-1111111", true},
		{"short running version, same commit", "main-07cd3bf", "main-07cd3bf", false},
		{"stable build moving to edge", "v1.0.0", "main-07cd3bf", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpdateAvailable(tt.current, &Release{Tag: tt.latest}); got != tt.want {
				t.Errorf("UpdateAvailable(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestRun_EdgeSkipsSignatureChecks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Inputs)
		code   string
		want   string
	}{
		{"edge has no checksums and is not blocked", func(in *Inputs) { in.ReleaseKind = KindEdgeImage; in.AssetNames = nil }, "release_signature", StatusSkipped},
		{"edge does not need a trust root", func(in *Inputs) {
			in.ReleaseKind = KindEdgeImage
			in.TrustRoot = nil
		}, "release_verifier", StatusSkipped},
		{"plain release still fails without checksums", func(in *Inputs) { in.ReleaseKind = KindRelease; in.AssetNames = nil }, "release_signature", StatusFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := baseInputs()
			tt.mutate(&in)
			checks := Run(context.Background(), in)
			got := byCode(checks, tt.code)
			if got.Status != tt.want {
				t.Fatalf("%s = %s (%s), want %s", tt.code, got.Status, got.Message, tt.want)
			}
			if tt.want == StatusSkipped && (got.Message != EdgeNoSignatureNote || Blocked(checks)) {
				t.Errorf("message = %q blocked = %v", got.Message, Blocked(checks))
			}
		})
	}
}
