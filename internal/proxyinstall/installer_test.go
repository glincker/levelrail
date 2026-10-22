package proxyinstall

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type fakeDocker struct {
	state   *docker.ContainerState
	locals  []docker.LocalContainer
	created []docker.ContainerSpec
	started []string
}

func (f *fakeDocker) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return f.state, nil
}

func (f *fakeDocker) Create(_ context.Context, s docker.ContainerSpec) (string, error) {
	f.created = append(f.created, s)
	f.state = &docker.ContainerState{ID: "id1", Name: s.Name}
	f.locals = append(f.locals, docker.LocalContainer{Name: s.Name, Labels: s.Labels})
	return "id1", nil
}

func (f *fakeDocker) Start(_ context.Context, id string) error {
	f.started = append(f.started, id)
	f.state.Running = true
	return nil
}

func (f *fakeDocker) ListLocalContainers(context.Context) ([]docker.LocalContainer, error) {
	return f.locals, nil
}

func baseInput() Input {
	return Input{Provider: ProviderTraefik, Brand: "Acme", DataDir: "/data"}
}

func TestBuildPlan(t *testing.T) {
	cases := []struct {
		name     string
		in       func(Input) Input
		wantErr  string
		contains []string
		absent   []string
	}{
		{name: "default", in: func(i Input) Input { return i }, contains: []string{`address: ":80"`, `address: ":443"`, "watch: true", "/etc/traefik/dynamic"}, absent: []string{"certificatesResolvers"}},
		{name: "public certs", in: func(i Input) Input { i.PublicCerts, i.ACMEEmail = true, "a@b.co"; return i }, contains: []string{"certificatesResolvers", `email: "a@b.co"`, "httpChallenge"}},
		{name: "certs without email stay off", in: func(i Input) Input { i.PublicCerts = true; return i }, absent: []string{"certificatesResolvers"}},
		{name: "nginx refused", in: func(i Input) Input { i.Provider = "nginx"; return i }, wantErr: "Only Traefik"},
		{name: "no data dir", in: func(i Input) Input { i.DataDir = ""; return i }, wantErr: "data directory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := BuildPlan(tc.in(baseInput()))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Container != "acme-traefik" || p.Restart != "unless-stopped" || p.Image != DefaultTraefikImage {
				t.Fatalf("unexpected plan %+v", p)
			}
			cfg := p.Files[0].Content
			for _, s := range tc.contains {
				if !strings.Contains(cfg, s) {
					t.Errorf("config missing %q:\n%s", s, cfg)
				}
			}
			for _, s := range tc.absent {
				if strings.Contains(cfg, s) {
					t.Errorf("config has %q:\n%s", s, cfg)
				}
			}
		})
	}
}

func TestBuildPlanImageOverride(t *testing.T) {
	in := baseInput()
	in.Image = "traefik:v9"
	p, err := BuildPlan(in)
	if err != nil || p.Image != "traefik:v9" {
		t.Fatalf("plan %+v err %v", p, err)
	}
}

func TestPlanRefusals(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*Installer, *fakeDocker)
		code   string
		inCont bool
	}{
		{name: "free host", setup: func(*Installer, *fakeDocker) {}},
		{name: "port 80 busy", setup: func(i *Installer, _ *fakeDocker) {
			i.PortHolder = func(_ context.Context, p int) string {
				if p == 80 {
					return "nginx"
				}
				return ""
			}
		}, code: CodePortBusy},
		{name: "port 443 busy", setup: func(i *Installer, _ *fakeDocker) {
			i.PortHolder = func(_ context.Context, p int) string {
				if p == 443 {
					return "apache2"
				}
				return ""
			}
		}, code: CodePortBusy},
		{name: "built-in ingress holds 443", setup: func(i *Installer, _ *fakeDocker) {
			i.OwnIngressPorts = func() []int { return []int{443} }
		}, code: CodeOwnIngress},
		{name: "foreign container with our name", setup: func(_ *Installer, f *fakeDocker) {
			f.state = &docker.ContainerState{ID: "x", Name: "acme-traefik"}
			f.locals = []docker.LocalContainer{{Name: "acme-traefik"}}
		}, code: CodeNameTaken},
		{name: "control plane in docker", inCont: true, setup: func(*Installer, *fakeDocker) {}, code: CodeInContainer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fd := &fakeDocker{}
			in := &Installer{Docker: fd, PortHolder: func(context.Context, int) string { return "" }, InContainer: tc.inCont}
			tc.setup(in, fd)
			p, err := in.Plan(context.Background(), baseInput())
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if p.Refusal != nil {
				got = p.Refusal.Code
			}
			if got != tc.code {
				t.Fatalf("refusal %q, want %q", got, tc.code)
			}
			if tc.code != "" {
				if _, err := in.Apply(context.Background(), baseInput()); err == nil {
					t.Fatal("apply must refuse")
				}
				if len(fd.created) != 0 {
					t.Fatal("refused apply created a container")
				}
			}
		})
	}
}

func TestApplyIdempotent(t *testing.T) {
	fd := &fakeDocker{}
	files := map[string]string{}
	in := &Installer{
		Docker: fd, PortHolder: func(context.Context, int) string { return "" },
		MkdirAll:  func(string, os.FileMode) error { return nil },
		WriteFile: func(p string, d []byte, _ os.FileMode) error { files[p] = string(d); return nil },
	}
	first, err := in.Apply(context.Background(), baseInput())
	if err != nil || !first.Created || !first.Started {
		t.Fatalf("first %+v err %v", first, err)
	}
	second, err := in.Apply(context.Background(), baseInput())
	if err != nil || second.Created {
		t.Fatalf("second %+v err %v", second, err)
	}
	if len(fd.created) != 1 || len(fd.started) != 1 {
		t.Fatalf("created %d started %d, want 1 and 1", len(fd.created), len(fd.started))
	}
	spec := fd.created[0]
	if spec.RestartPolicy != "unless-stopped" || len(spec.Ports) != 2 || spec.Labels["acme.managed-by"] != "acme" {
		t.Fatalf("spec %+v", spec)
	}
	if _, ok := files["/data/proxy/traefik/traefik.yml"]; !ok {
		t.Fatalf("static config not written: %v", files)
	}
}

func TestApplyStartsStoppedOwnContainer(t *testing.T) {
	fd := &fakeDocker{
		state:  &docker.ContainerState{ID: "id9", Name: "acme-traefik"},
		locals: []docker.LocalContainer{{Name: "acme-traefik", Labels: map[string]string{"acme.managed-by": "acme"}}},
	}
	in := &Installer{Docker: fd, PortHolder: func(context.Context, int) string { return "" }}
	res, err := in.Apply(context.Background(), baseInput())
	if err != nil || !res.Started || res.Created || len(fd.started) != 1 || fd.started[0] != "id9" {
		t.Fatalf("res %+v err %v started %v", res, err, fd.started)
	}
}
