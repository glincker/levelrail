package catalog

import (
	"os"
	"regexp"
	"testing"
)

func TestVerified_RecordsMatchCatalog(t *testing.T) {
	for id, rec := range VerifiedData() {
		tpl, ok := TemplateByID(id)
		if !ok {
			t.Errorf("verified id %q is not in the catalog", id)
			continue
		}
		if rec.Tag == "" || len(rec.Images) == 0 || len(rec.Arches) == 0 {
			t.Errorf("verified %q needs tag, images and arches: %+v", id, rec)
		}
		if _, ok := VerifiedFor(tpl); !ok {
			t.Errorf("verified %q records images that no longer match the pinned tags; rerun the smoke for it", id)
		}
	}
	if IsVerified("definitely-not-a-template") {
		t.Error("unknown id reported verified")
	}
}

// A record is a fact about pinned tags, so changing a tag must drop it.
func TestVerifiedFor_DropsOnTagChange(t *testing.T) {
	tpl := Template{ID: "x", Compose: "services:\n  a:\n    image: foo/a:2\n"}
	verifiedData["x"] = VerifiedRecord{Tag: "1", Images: []string{"foo/a:1"}, Arches: []string{"amd64"}}
	defer delete(verifiedData, "x")
	if _, ok := VerifiedFor(tpl); ok {
		t.Fatal("record for tag 1 must not verify tag 2")
	}
	tpl.Compose = "services:\n  a:\n    image: foo/a:1\n"
	if _, ok := VerifiedFor(tpl); !ok {
		t.Fatal("record must verify the same pinned tag")
	}
}

// The status page is the human record of the same runs, so every record must be a pass there.
func TestVerified_RecordsArePassesInStatusPage(t *testing.T) {
	body, err := os.ReadFile("../../docs/templates-status.md")
	if err != nil {
		t.Skip("docs not present")
	}
	pass := regexp.MustCompile("(?m)^\\| `([^`]+)` \\| pass \\|")
	got := map[string]bool{}
	for _, m := range pass.FindAllStringSubmatch(string(body), -1) {
		got[m[1]] = true
	}
	for id := range VerifiedData() {
		if !got[id] {
			t.Errorf("%q is in verified.json but not a pass in docs/templates-status.md", id)
		}
	}
}
