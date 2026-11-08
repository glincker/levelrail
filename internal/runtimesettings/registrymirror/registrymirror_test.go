package registrymirror

import "testing"

func TestApply(t *testing.T) {
	const dg = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct{ name, ref, prefix, want string }{
		{"empty prefix", "ghcr.io/glincker/levelrail-agent:v1", "", "ghcr.io/glincker/levelrail-agent:v1"},
		{"tag", "ghcr.io/glincker/levelrail-agent:v1", "m.daocloud.io", "m.daocloud.io/ghcr.io/glincker/levelrail-agent:v1"},
		{"digest kept", "docker.io/moby/buildkit:v0.34.1-rootless" + dg, "m.daocloud.io", "m.daocloud.io/docker.io/moby/buildkit:v0.34.1-rootless" + dg},
		{"path prefix", "ghcr.io/a/b:edge", "mirror.example.com:5000/proxy", "mirror.example.com:5000/proxy/ghcr.io/a/b:edge"},
		{"idempotent", "m.daocloud.io/ghcr.io/a/b:edge", "m.daocloud.io", "m.daocloud.io/ghcr.io/a/b:edge"},
		{"invalid prefix ignored", "ghcr.io/a/b:edge", "https://x.io", "ghcr.io/a/b:edge"},
		{"empty ref", "", "m.daocloud.io", ""},
	}
	for _, c := range cases {
		if got := Apply(c.ref, c.prefix); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := []string{"", "m.daocloud.io", "mirror.example.com:5000/proxy", "localhost"}
	bad := []string{"https://m.daocloud.io", "m.daocloud.io/", "bad host", "-x.io", "a//b"}
	for _, v := range ok {
		if err := Validate(v); err != nil {
			t.Errorf("%q should be valid: %v", v, err)
		}
	}
	for _, v := range bad {
		if err := Validate(v); err == nil {
			t.Errorf("%q should be invalid", v)
		}
	}
}
