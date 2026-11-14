package api

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The dashboard types the repository list's provider as a string union; a backend
// provider missing from it produced undefined rows in the New app wizard.
func TestWebGitRepoProviderUnionMatchesBackend(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "types", "gitRepos.ts"))
	if err != nil {
		t.Fatalf("read web/src/types/gitRepos.ts: %v", err)
	}
	decl := regexp.MustCompile(`(?s)export type GitRepoProvider\s*=(.*?)\n\s*\n`).FindSubmatch(src)
	if decl == nil {
		t.Fatal("GitRepoProvider union declaration not found in web/src/types/gitRepos.ts")
	}
	have := map[string]bool{}
	for _, m := range regexp.MustCompile(`'([a-z_]+)'`).FindAllSubmatch(decl[1], -1) {
		have[string(m[1])] = true
	}
	want := map[string]bool{gitProviderGitHub: true}
	for _, p := range connectionGitProviders {
		want[p] = true
	}
	var missing, extra []string
	for p := range want {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	for p := range have {
		if !want[p] {
			extra = append(extra, p)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("web GitRepoProvider is out of step with the backend: missing from the union: [%s]; not served by the backend: [%s]. Update web/src/types/gitRepos.ts and fromGitRepoItem.",
			strings.Join(missing, ", "), strings.Join(extra, ", "))
	}
}
