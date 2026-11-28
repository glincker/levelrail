package api

import (
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestCIStackTrustMatchesRepositoryIDs(t *testing.T) {
	claims := CIClaims{Repository: "acme/web", RepositoryID: "11", RepositoryOwnerID: "22", Ref: "refs/heads/main"}
	base := store.CIStackTrust{Repository: "acme/web", RefPattern: "refs/heads/main"}
	with := func(repoID, ownerID string) store.CIStackTrust {
		tr := base
		tr.RepositoryID, tr.RepositoryOwnerID = repoID, ownerID
		return tr
	}
	cases := []struct {
		name  string
		trust store.CIStackTrust
		want  bool
	}{
		{"name and ids match", with("11", "22"), true},
		{"repository id differs", with("99", "22"), false},
		{"owner id differs", with("11", "99"), false},
		{"no stored ids falls back to name", base, true},
	}
	for _, tc := range cases {
		if got := stackTrustMatches(tc.trust, claims); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
