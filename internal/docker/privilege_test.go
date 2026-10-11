package docker

import "testing"

func TestParseSecurityOptions(t *testing.T) {
	tests := []struct {
		name         string
		opts         []string
		wantRootless bool
		wantUserns   bool
	}{
		{name: "rootful default", opts: []string{"name=apparmor", "name=seccomp,profile=builtin", "name=cgroupns"}},
		{name: "rootless", opts: []string{"name=seccomp,profile=builtin", "name=rootless", "name=cgroupns"}, wantRootless: true},
		{name: "userns remap", opts: []string{"name=seccomp,profile=builtin", "name=userns"}, wantUserns: true},
		{name: "empty", opts: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSecurityOptions(tt.opts)
			if got.Rootless != tt.wantRootless || got.UsernsRemap != tt.wantUserns {
				t.Fatalf("ParseSecurityOptions(%v) = %+v", tt.opts, got)
			}
		})
	}
}

type recordingDeclarer struct {
	got      []CreateDeclaration
	released int
}

func (r *recordingDeclarer) DeclareCreate(d CreateDeclaration) func() {
	r.got = append(r.got, d)
	return func() { r.released++ }
}

func TestDeclareCarriesSpecGrants(t *testing.T) {
	rec := &recordingDeclarer{}
	c := &Client{declarer: rec}
	release := c.declare(ContainerSpec{
		Name: "web", NetworkMode: "host", GPU: &GPURequest{Count: 1}, CapAdd: []string{"NET_ADMIN"},
		BindMounts: []BindMount{{HostPath: "/srv/data", ContainerPath: "/data"}},
	})
	release()
	if len(rec.got) != 1 || rec.released != 1 {
		t.Fatalf("declarations = %+v released = %d", rec.got, rec.released)
	}
	d := rec.got[0]
	if d.Name != "web" || !d.HostNetwork || !d.GPU || len(d.BindPaths) != 1 || d.BindPaths[0] != "/srv/data" || d.CapAdd[0] != "NET_ADMIN" {
		t.Fatalf("declaration = %+v", d)
	}
	if (&Client{}).declare(ContainerSpec{Name: "x"}) == nil {
		t.Fatal("no declarer must still return a release func")
	}
}
