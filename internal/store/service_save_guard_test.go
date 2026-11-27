package store

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// uncheckedSaveCalls lists the non-test files that reference a desired-service save
// without SaveRebased or CheckRev on the same line, with how many such lines each may hold.
// A new caller must either go through SaveRebased or be added here with a reason.
var uncheckedSaveCalls = map[string]struct {
	count  int
	reason string
}{
	"internal/demo/seed_apps.go":                 {1, "seeds new demo rows"},
	"internal/demo/seed_pages.go":                {1, "seeds new demo rows"},
	"internal/deploy/order.go":                   {2, "save funcs handed to saveRebasedFromSpec"},
	"internal/deploy/rollback.go":                {2, "save funcs handed to SaveRebased"},
	"internal/api/apps_create.go":                {1, "creates a new row"},
	"internal/api/apps_clone.go":                 {1, "creates a new row"},
	"internal/api/canary.go":                     {1, "creates the canary row"},
	"internal/api/platform_import.go":            {2, "creates an imported app that must not exist yet"},
	"internal/api/routes_project_clone_apply.go": {2, "creates a cloned row"},
	"internal/api/environment_clone.go":          {1, "creates a cloned row"},
	"internal/api/recently_deleted.go":           {1, "restores a row that must not exist"},
	"internal/api/apps_compose.go":               {2, "save funcs handed to saveComposeService"},
	"internal/api/deploys.go":                    {1, "wrapper that ImageTrigger saves through SaveRebased"},
	"internal/deployurls/deployurls.go":          {1, "creates the suspended deploy URL copy"},
	"internal/deployurls/save_domains.go":        {1, "fallback for a store that cannot re-read"},
}

var saveRef = regexp.MustCompile(`\.Save(Suspended)?DesiredService(Ordered)?\b`)

func TestDesiredServiceSavesAreRevisionChecked(t *testing.T) {
	found := map[string]int{}
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, "../../"))
			if strings.HasPrefix(rel, "internal/store/") {
				return nil
			}
			data, err := os.ReadFile(path) //nolint:gosec // walks this repo's own source tree
			if err != nil {
				return err
			}
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") || !saveRef.MatchString(line) {
					continue
				}
				if strings.Contains(line, "SaveRebased(") || strings.Contains(line, "CheckRev") {
					continue
				}
				found[rel]++
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for file, n := range found {
		allowed, ok := uncheckedSaveCalls[file]
		if !ok || n > allowed.count {
			t.Errorf("%s has %d desired-service save call(s) without SaveRebased or CheckRev: convert it or list it in uncheckedSaveCalls with a reason", file, n)
		}
	}
	for file, allowed := range uncheckedSaveCalls {
		if found[file] != allowed.count {
			t.Errorf("%s lists %d unchecked save call(s) (%s) but has %d: update uncheckedSaveCalls", file, allowed.count, allowed.reason, found[file])
		}
	}
}
