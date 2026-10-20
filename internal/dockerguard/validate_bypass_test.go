package dockerguard

import (
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestValidateCreateBindBypassAttempts(t *testing.T) {
	declared := t.TempDir()
	dotted := declared + "/../" + filepath.Base(declared)
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"double slash socket", "//var//run/docker.sock", RuleBindSensitive},
		{"dot segment socket", "/var/run/./docker.sock", RuleBindSensitive},
		{"traversal to socket", "/srv/app/../../var/run/docker.sock", RuleBindSensitive},
		{"run alias of socket", "/run/docker.sock", RuleBindSensitive},
		{"trailing slash etc", "/etc/", RuleBindSensitive},
		{"etc via parent", "/etc/ssh/..", RuleBindSensitive},
		{"proc self root", "/proc/self/root", RuleBindSensitive},
		{"docker lib", "/var/lib/docker/overlay2", RuleBindSensitive},
		{"ancestor var lib", "/var/lib", RuleBindSensitive},
		{"leading space is not absolute", " /etc", RuleBindUndeclared},
		{"tilde is not absolute", "~/.ssh", RuleBindUndeclared},
		{"relative traversal", "../../etc", RuleBindUndeclared},
		{"undeclared lookalike", declared + "x", RuleBindUndeclared},
		{"declared with dot segment is cleaned and allowed", dotted, ""},
	}
	grant := docker.CreateDeclaration{Name: "app", BindPaths: []string{declared}}
	for _, tt := range tests {
		for _, form := range []string{"mount", "bind-string"} {
			t.Run(tt.name+"/"+form, func(t *testing.T) {
				p := testPolicy(t)
				hc := container.HostConfig{}
				if form == "mount" {
					hc.Mounts = []mount.Mount{{Type: mount.TypeBind, Source: tt.src, Target: "/p"}}
				} else {
					hc.Binds = []string{tt.src + ":/p"}
				}
				got := p.validateCreate(container.CreateRequest{HostConfig: &hc}, grant)
				if form == "bind-string" && len(tt.src) > 0 && tt.src[0] != '/' {
					return
				}
				switch {
				case tt.want == "" && len(got) > 0:
					t.Fatalf("want allowed, got %v", rulesOf(got))
				case tt.want != "" && (len(got) == 0 || got[0].Rule != tt.want):
					t.Fatalf("want %s, got %v", tt.want, rulesOf(got))
				}
			})
		}
	}
}

func TestValidateCreateRejectsEveryHostEscapeTogether(t *testing.T) {
	p := testPolicy(t)
	hc := container.HostConfig{
		Privileged: true, PidMode: "HOST", IpcMode: "host", NetworkMode: "Host",
		CapAdd: []string{"cap_sys_admin"}, SecurityOpt: []string{"Seccomp=Unconfined"},
		Binds: []string{"/:/host"},
	}
	got := rulesOf(p.validateCreate(container.CreateRequest{HostConfig: &hc}, docker.CreateDeclaration{}))
	for _, want := range []string{RulePrivileged, RuleHostPID, RuleHostIPC, RuleNetworkHost, RuleCapAdd, RuleSecurityOpt, RuleBindSensitive} {
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}
