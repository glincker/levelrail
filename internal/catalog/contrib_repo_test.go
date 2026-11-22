package catalog

import "testing"

func TestRepoComposeCandidates(t *testing.T) {
	got, err := RepoComposeCandidates("https://github.com/acme/app/tree/main/deploy")
	if err != nil || len(got) != 4 || got[0] != "https://raw.githubusercontent.com/acme/app/main/deploy/compose.yaml" {
		t.Fatalf("subpath = %v, %v", got, err)
	}
	got, err = RepoComposeCandidates("https://github.com/acme/app.git")
	if err != nil || got[0] != "https://raw.githubusercontent.com/acme/app/HEAD/compose.yaml" {
		t.Fatalf("root = %v, %v", got, err)
	}
	for _, bad := range []string{"http://github.com/a/b", "https://example.com/a/b", "https://github.com/a", "https://github.com/a/b/tree/main/../x"} {
		if _, err := RepoComposeCandidates(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
