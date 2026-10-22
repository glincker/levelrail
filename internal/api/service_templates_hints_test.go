package api

import "testing"

func TestTemplateResourceHints(t *testing.T) {
	cases := []struct {
		name, body string
		db, vol    bool
	}{
		{"plain", "services:\n  web:\n    image: nginx:1\n", false, false},
		{"db and volume", "services:\n  db:\n    image: postgres:16\n    volumes:\n      - data:/var/lib/postgresql\n", true, true},
		{"registry path", "services:\n  c:\n    image: docker.io/library/redis:7\n", true, false},
		{"name only", "services:\n  web:\n    image: ghost:5\n    environment:\n      DB: mysql\n", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, vol := templateResourceHints(c.body)
			if db != c.db || vol != c.vol {
				t.Fatalf("got db=%v vol=%v, want db=%v vol=%v", db, vol, c.db, c.vol)
			}
		})
	}
}
