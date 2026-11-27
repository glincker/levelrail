package sources

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"testing/fstest"
)

func fakeLookup(m map[string][]string) LookupFunc {
	return func(_ context.Context, host string) ([]netip.Addr, error) {
		var out []netip.Addr
		for _, s := range m[host] {
			out = append(out, netip.MustParseAddr(s))
		}
		return out, nil
	}
}

func TestCheckURL(t *testing.T) {
	look := fakeLookup(map[string][]string{
		"git.example.com": {"93.184.216.34"},
		"rebind.example":  {"93.184.216.34", "10.0.0.5"},
		"meta.example":    {"169.254.169.254"},
		"loop6.example":   {"::1"},
	})
	tests := []struct {
		name    string
		url     string
		wantSSH bool
		wantErr error
	}{
		{"https public", "https://git.example.com/acme/t.git", false, nil},
		{"ssh scheme", "ssh://git@git.example.com/acme/t.git", true, nil},
		{"scp style", "git@git.example.com:acme/t.git", true, nil},
		{"http refused", "http://git.example.com/a.git", false, ErrURLScheme},
		{"file refused", "file:///etc/passwd", false, ErrURLInvalid},
		{"git protocol refused", "git://git.example.com/a.git", false, ErrURLScheme},
		{"https creds refused", "https://user:tok@git.example.com/a.git", false, ErrURLCreds},
		{"ssh password refused", "ssh://git:pw@git.example.com/a.git", false, ErrURLCreds},
		{"loopback literal", "https://127.0.0.1/a.git", false, ErrURLInternal},
		{"private literal", "https://10.1.2.3/a.git", false, ErrURLInternal},
		{"link-local literal", "https://169.254.169.254/a.git", false, ErrURLInternal},
		{"ipv6 loopback literal", "https://[::1]/a.git", false, ErrURLInternal},
		{"cgnat literal", "https://100.64.0.1/a.git", false, ErrURLInternal},
		{"localhost", "https://localhost/a.git", false, ErrURLInternal},
		{"scp private ip", "git@192.168.1.5:acme/t.git", false, ErrURLInternal},
		{"dns to metadata", "https://meta.example/a.git", false, ErrURLInternal},
		{"dns rebinding mix", "https://rebind.example/a.git", false, ErrURLInternal},
		{"dns to v6 loopback", "https://loop6.example/a.git", false, ErrURLInternal},
		{"unresolvable", "https://nope.example/a.git", false, ErrURLInvalid},
		{"empty", "", false, ErrURLInvalid},
		{"space", "https://git.example.com/a b.git", false, ErrURLInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ssh, err := CheckURL(context.Background(), tc.url, look)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && ssh != tc.wantSSH {
				t.Fatalf("ssh = %v, want %v", ssh, tc.wantSSH)
			}
		})
	}
}

func TestNamespacedIDCollisions(t *testing.T) {
	builtin := func(id string) bool { return id == "ghost" || id == "acme/ghost" }
	tests := []struct {
		source, id string
		want       string
		wantErr    bool
	}{
		{"acme", "internal-wiki", "acme/internal-wiki", false},
		{"acme", "ghost", "", true},
		{"Acme", "app", "", true},
		{"acme_x", "app", "", true},
		{"", "app", "", true},
	}
	for _, tc := range tests {
		got, err := NamespacedID(tc.source, tc.id, builtin)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("NamespacedID(%q,%q) = %q, %v", tc.source, tc.id, got, err)
		}
	}
}

func TestValidRef(t *testing.T) {
	for ref, want := range map[string]bool{"": true, "main": true, "release/v1": true, "-x": false, "a..b": false, "a b": false, "$(x)": false} {
		if ValidRef(ref) != want {
			t.Errorf("ValidRef(%q) = %v", ref, !want)
		}
	}
}

const goodTemplate = `id: wiki
name: Wiki
description: Team wiki
category: tools
documentation_url: https://example.com/docs
recommended_memory_bytes: 268435456
`

const goodCompose = "services:\n  wiki:\n    image: example/wiki:1.2.3\n    volumes:\n      - wiki-data:/data\nvolumes:\n  wiki-data: {}\n"

func TestBuildNamespacesAndRefusesBadTemplates(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/wiki/template.yaml":   {Data: []byte(goodTemplate)},
		"templates/wiki/compose.yaml":    {Data: []byte(goodCompose)},
		"templates/broken/template.yaml": {Data: []byte("id: broken\n")},
		"templates/broken/compose.yaml":  {Data: []byte("services: {}\n")},
		"templates/bind/template.yaml":   {Data: []byte("id: bind\nname: Bind\ndescription: d\ncategory: tools\ndocumentation_url: https://example.com\nrecommended_memory_bytes: 1048576\n")},
		"templates/bind/compose.yaml":    {Data: []byte("services:\n  a:\n    image: example/a:1.0.0\n    volumes:\n      - /etc:/etc\n")},
	}
	res := Build("acme", fsys, func(string) bool { return false })
	if len(res.Entries) != 1 || res.Entries[0].ID != "acme/wiki" || res.Entries[0].Source != "acme" {
		t.Fatalf("entries = %+v, skipped = %v", res.Entries, res.Skipped)
	}
	if len(res.Skipped) != 2 {
		t.Fatalf("skipped = %v, want broken and bind", res.Skipped)
	}
	res = Build("acme", fsys, func(id string) bool { return id == "wiki" })
	if len(res.Entries) != 0 {
		t.Fatalf("built-in collision was not refused: %+v", res.Entries)
	}
}
