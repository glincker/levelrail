package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func TestDefaultFetchLatestRelease(t *testing.T) {
	orig := fetchStableRelease
	t.Cleanup(func() { fetchStableRelease = orig })

	tests := []struct {
		name    string
		rel     *upgrade.Release
		err     error
		wantTag string
		wantNil bool
		wantErr bool
		assets  int
	}{
		{name: "product release with assets", rel: &upgrade.Release{Tag: "v0.2.0", URL: "https://example.test/r", AssetNames: []string{"levelrail-linux-amd64", "checksums.txt"}}, wantTag: "v0.2.0", assets: 2},
		{name: "no release published", rel: nil, wantNil: true},
		{name: "fetch failure is wrapped", err: errors.New("boom"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetchStableRelease = func(context.Context, string) (*upgrade.Release, error) { return tc.rel, tc.err }
			got, err := defaultFetchLatestRelease(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantNil || tc.wantErr {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil || got.TagName != tc.wantTag || len(got.Assets) != tc.assets {
				t.Fatalf("got %+v, want tag %s with %d assets", got, tc.wantTag, tc.assets)
			}
		})
	}
}
