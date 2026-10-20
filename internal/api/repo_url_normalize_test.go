package api

import "testing"

func TestNormalizePastedRepoURL(t *testing.T) {
	cases := []struct{ in, url, ref, dir string }{
		{"https://github.com/acme/shop", "https://github.com/acme/shop", "", ""},
		{"https://github.com/acme/shop.git/", "https://github.com/acme/shop", "", ""},
		{"git@github.com:acme/shop.git", "https://github.com/acme/shop", "", ""},
		{"ssh://git@gitlab.com/acme/shop.git", "https://gitlab.com/acme/shop", "", ""},
		{"https://github.com/acme/shop/tree/dev/apps/web?tab=1", "https://github.com/acme/shop", "dev", "apps/web"},
		{"https://gitea.example.com/acme/shop#readme", "https://gitea.example.com/acme/shop", "", ""},
	}
	for _, c := range cases {
		u, ref, dir := normalizePastedRepoURL(c.in)
		if u != c.url || ref != c.ref || dir != c.dir {
			t.Errorf("%q = %q %q %q, want %q %q %q", c.in, u, ref, dir, c.url, c.ref, c.dir)
		}
	}
}
