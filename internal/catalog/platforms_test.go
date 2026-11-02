package catalog

import "testing"

// A template may only claim platforms that every one of its images lists in the scan file.
func TestPlatforms_NoUnbackedClaims(t *testing.T) {
	data := PlatformsData()
	byRef := map[string]ImagePlatforms{}
	for _, p := range data.Images {
		byRef[p.Ref()] = p
	}
	for id, claimed := range data.Templates {
		tpl, ok := TemplateByID(id)
		if !ok {
			t.Errorf("platforms.json template %q is not in the catalog", id)
			continue
		}
		imgs, _ := ComposeImages(tpl.Compose)
		if len(imgs) == 0 {
			t.Errorf("%s: claims platforms but has no literal images", id)
		}
		for _, ref := range imgs {
			p, found := byRef[ref]
			if !found || p.Error != "" {
				t.Errorf("%s: image %s is not verified in platforms.json", id, ref)
				continue
			}
			for _, c := range claimed {
				if !hasString(p.Platforms, c) {
					t.Errorf("%s claims %s but %s lists %v", id, c, ref, p.Platforms)
				}
			}
		}
		if got := tpl.Platforms; len(got) != len(claimed) {
			t.Errorf("%s: loader did not merge platforms", id)
		}
	}
}

func hasString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func TestSplitImageRef(t *testing.T) {
	for in, want := range map[string][2]string{
		"postgres:16":        {"postgres", "16"},
		"localhost:5000/app": {"localhost:5000/app", "latest"},
		"ghcr.io/a/b:v1":     {"ghcr.io/a/b", "v1"},
		"nginx":              {"nginx", "latest"},
		"a/b@sha256:abc":     {"a/b@sha256:abc", ""},
	} {
		if i, tg := SplitImageRef(in); i != want[0] || tg != want[1] {
			t.Errorf("%s -> %s %s", in, i, tg)
		}
	}
}
