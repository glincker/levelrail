package build

import "testing"

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
