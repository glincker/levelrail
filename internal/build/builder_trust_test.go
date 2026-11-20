package build

import (
	"context"
	"testing"
)

func TestChooseBuildNodeBuilderTrust(t *testing.T) {
	laptop := NodeInfo{ID: "n-laptop", Name: "laptop", AcceptsBuildWorkloads: true, Online: true, Trust: TrustPersonal}
	server := NodeInfo{ID: "n-z-server", Name: "srv", AcceptsBuildWorkloads: true, Online: true}
	nodes := []NodeInfo{laptop, server}
	cases := []struct {
		name string
		q    Query
		p    Policy
		want string
	}{
		{"preview may use a personal builder", Query{EnvKind: "preview"}, Policy{}, "n-laptop"},
		{"production skips the personal builder", Query{EnvKind: "production"}, Policy{}, "n-z-server"},
		{"setting allows personal in production", Query{EnvKind: "production"}, Policy{PersonalInProduction: true}, "n-laptop"},
		{"explicit node id beats the default", Query{EnvKind: "production", AllowedBuilders: []string{"n-laptop"}}, Policy{}, "n-laptop"},
		{"explicit tier list excludes personal", Query{EnvKind: "preview", AllowedBuilders: []string{"server"}}, Policy{}, "n-z-server"},
		{"pinned personal on production falls back", Query{EnvKind: "production", Pinned: "n-laptop"}, Policy{}, ""},
	}
	for _, c := range cases {
		if got := ChooseBuildNode(nodes, c.q, c.p).Node; got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestTrustMatchesTierOrIDNeverName(t *testing.T) {
	// A node named like a tier word must not satisfy a tier allow-list.
	alias := NodeInfo{ID: "n-a", Name: "server", AcceptsBuildWorkloads: true, Online: true, Trust: TrustPersonal}
	byName := NodeInfo{ID: "n-b", Name: "n-c", AcceptsBuildWorkloads: true, Online: true, Trust: TrustPersonal}
	q := Query{EnvKind: "preview", AllowedBuilders: []string{"server"}}
	if d := ChooseBuildNode([]NodeInfo{alias}, q, Policy{}); d.Node != "" {
		t.Fatalf("tier-word name matched: %+v", d)
	}
	q = Query{EnvKind: "preview", AllowedBuilders: []string{"n-b"}}
	if d := ChooseBuildNode([]NodeInfo{alias, byName}, q, Policy{}); d.Node != "n-b" || d.Trust != TrustPersonal {
		t.Fatalf("id match failed: %+v", d)
	}
	// A pin by name never resolves.
	if d := ChooseBuildNode([]NodeInfo{alias}, Query{EnvKind: "preview", Pinned: "server"}, Policy{}); d.Node != "" {
		t.Fatalf("pin by name resolved: %+v", d)
	}
}

func TestUnknownEnvironmentUsesServerBuildersOnly(t *testing.T) {
	laptop := NodeInfo{ID: "a", Name: "laptop", AcceptsBuildWorkloads: true, Online: true, Trust: TrustPersonal}
	srv := NodeInfo{ID: "z", Name: "srv", AcceptsBuildWorkloads: true, Online: true}
	if d := ChooseBuildNode([]NodeInfo{laptop, srv}, Query{}, Policy{}); d.Node != "z" {
		t.Fatalf("unknown app placed on %+v", d)
	}
	if d := ChooseBuildNode([]NodeInfo{laptop}, Query{}, Policy{}); d.Node != "" {
		t.Fatalf("unknown app reached a personal builder: %+v", d)
	}
}

func TestAppForPrefersContextApp(t *testing.T) {
	if got := AppFor(WithApp(context.Background(), "web"), "h/img:t"); got != "web" {
		t.Fatalf("got %q", got)
	}
	if got := AppFor(context.Background(), "h/img:t"); got != "img" {
		t.Fatalf("got %q", got)
	}
}
