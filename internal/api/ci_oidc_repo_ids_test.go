package api

import (
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestCITrustMatchesRepositoryIDs(t *testing.T) {
	claims := CIClaims{Repository: "acme/web", RepositoryID: "11", RepositoryOwnerID: "22", Ref: "refs/heads/main"}
	cases := []struct {
		name  string
		trust store.CIOIDCTrust
		want  bool
	}{
		{"name and ids match", store.CIOIDCTrust{Repository: "acme/web", RefPattern: "refs/heads/main", RepositoryID: "11", RepositoryOwnerID: "22"}, true},
		{"repository id differs", store.CIOIDCTrust{Repository: "acme/web", RefPattern: "refs/heads/main", RepositoryID: "99", RepositoryOwnerID: "22"}, false},
		{"owner id differs", store.CIOIDCTrust{Repository: "acme/web", RefPattern: "refs/heads/main", RepositoryID: "11", RepositoryOwnerID: "99"}, false},
		{"no stored ids falls back to name", store.CIOIDCTrust{Repository: "acme/web", RefPattern: "refs/heads/main"}, true},
		{"name differs", store.CIOIDCTrust{Repository: "acme/other", RefPattern: "refs/heads/main", RepositoryID: "11", RepositoryOwnerID: "22"}, false},
	}
	for _, tc := range cases {
		if got := ciTrustMatches(tc.trust, claims); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
