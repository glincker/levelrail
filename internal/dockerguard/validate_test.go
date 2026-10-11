package dockerguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func testPolicy(t *testing.T) Policy {
	t.Helper()
	return PolicyFromHardening(docker.HardeningConfig{ExtraCaps: []string{"SYS_PTRACE"}}, Tunables{}, "/srv/levelrail-test-data")
}

func rulesOf(vs []Violation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Rule)
	}
	return out
}

func TestValidateCreate(t *testing.T) {
	declaredDir := t.TempDir()
	gpu := docker.CreateDeclaration{Name: "gpu-app", GPU: true}
	sidecar := docker.CreateDeclaration{Name: "egress", CapAdd: []string{"NET_ADMIN", "NET_RAW"}}
	hostNet := docker.CreateDeclaration{Name: "hn", HostNetwork: true}
	binds := docker.CreateDeclaration{Name: "app", BindPaths: []string{declaredDir}}

	tests := []struct {
		name     string
		hc       container.HostConfig
		grant    docker.CreateDeclaration
		allowNet bool
		want     string
	}{
		{name: "hardened default allowed", hc: container.HostConfig{CapDrop: []string{"ALL"}, CapAdd: docker.MinimalCapabilities, SecurityOpt: []string{"no-new-privileges"}}},
		{name: "privileged denied", hc: container.HostConfig{Privileged: true}, want: RulePrivileged},
		{name: "host pid denied", hc: container.HostConfig{PidMode: "host"}, want: RuleHostPID},
		{name: "container pid allowed", hc: container.HostConfig{PidMode: "container:abc"}},
		{name: "host ipc denied", hc: container.HostConfig{IpcMode: "host"}, want: RuleHostIPC},
		{name: "private ipc allowed", hc: container.HostConfig{IpcMode: "private"}},
		{name: "host uts denied", hc: container.HostConfig{UTSMode: "HOST"}, want: RuleHostUTS},
		{name: "host userns denied", hc: container.HostConfig{UsernsMode: "host"}, want: RuleHostUserns},
		{name: "host cgroupns denied", hc: container.HostConfig{CgroupnsMode: "host"}, want: RuleHostCgroupns},
		{name: "host network undeclared denied", hc: container.HostConfig{NetworkMode: "host"}, allowNet: true, want: RuleNetworkHost},
		{name: "host network declared but not enabled denied", hc: container.HostConfig{NetworkMode: "host"}, grant: hostNet, want: RuleNetworkHost},
		{name: "host network declared and enabled allowed", hc: container.HostConfig{NetworkMode: "host"}, grant: hostNet, allowNet: true},
		{name: "container network allowed", hc: container.HostConfig{NetworkMode: "container:app"}},
		{name: "named network allowed", hc: container.HostConfig{NetworkMode: "my-net"}},
		{name: "operator extra cap allowed", hc: container.HostConfig{CapAdd: []string{"CAP_SYS_PTRACE"}}},
		{name: "sys admin denied", hc: container.HostConfig{CapAdd: []string{"SYS_ADMIN"}}, want: RuleCapAdd},
		{name: "cap all denied", hc: container.HostConfig{CapAdd: []string{"ALL"}}, want: RuleCapAdd},
		{name: "net admin undeclared denied", hc: container.HostConfig{CapAdd: []string{"NET_ADMIN"}}, want: RuleCapAdd},
		{name: "net admin declared allowed", hc: container.HostConfig{CapAdd: []string{"net_admin", "NET_RAW"}}, grant: sidecar},
		{name: "declared non grantable cap still denied", hc: container.HostConfig{CapAdd: []string{"SYS_MODULE"}}, grant: docker.CreateDeclaration{CapAdd: []string{"SYS_MODULE"}}, want: RuleCapAdd},
		{name: "gpu request undeclared denied", hc: container.HostConfig{Resources: container.Resources{DeviceRequests: []container.DeviceRequest{{Driver: "nvidia", Count: -1}}}}, want: RuleDevices},
		{name: "gpu request declared allowed", hc: container.HostConfig{Resources: container.Resources{DeviceRequests: []container.DeviceRequest{{Driver: "nvidia", Count: -1}}}}, grant: gpu},
		{name: "raw device denied", hc: container.HostConfig{Resources: container.Resources{Devices: []container.DeviceMapping{{PathOnHost: "/dev/sda"}}}}, grant: gpu, want: RuleDevices},
		{name: "nvidia device for gpu app allowed", hc: container.HostConfig{Resources: container.Resources{Devices: []container.DeviceMapping{{PathOnHost: "/dev/nvidia0"}}}}, grant: gpu},
		{name: "nvidia device without gpu denied", hc: container.HostConfig{Resources: container.Resources{Devices: []container.DeviceMapping{{PathOnHost: "/dev/nvidia0"}}}}, want: RuleDevices},
		{name: "device cgroup rules denied", hc: container.HostConfig{Resources: container.Resources{DeviceCgroupRules: []string{"c *:* rwm"}}}, grant: gpu, want: RuleDeviceCgroupRules},
		{name: "seccomp unconfined denied", hc: container.HostConfig{SecurityOpt: []string{"seccomp=unconfined"}}, want: RuleSecurityOpt},
		{name: "seccomp colon form denied", hc: container.HostConfig{SecurityOpt: []string{"seccomp:unconfined"}}, want: RuleSecurityOpt},
		{name: "custom seccomp profile denied", hc: container.HostConfig{SecurityOpt: []string{`seccomp={"defaultAction":"SCMP_ACT_ALLOW"}`}}, want: RuleSecurityOpt},
		{name: "apparmor unconfined denied", hc: container.HostConfig{SecurityOpt: []string{"apparmor=unconfined"}}, want: RuleSecurityOpt},
		{name: "apparmor named profile allowed", hc: container.HostConfig{SecurityOpt: []string{"apparmor=docker-default"}}},
		{name: "selinux disable denied", hc: container.HostConfig{SecurityOpt: []string{"label=disable"}}, want: RuleSecurityOpt},
		{name: "selinux spc_t denied", hc: container.HostConfig{SecurityOpt: []string{"label=type:spc_t"}}, want: RuleSecurityOpt},
		{name: "systempaths unconfined denied", hc: container.HostConfig{SecurityOpt: []string{"systempaths=unconfined"}}, want: RuleSecurityOpt},
		{name: "no new privileges false denied", hc: container.HostConfig{SecurityOpt: []string{"no-new-privileges:false"}}, want: RuleSecurityOpt},
		{name: "masked paths override denied", hc: container.HostConfig{MaskedPaths: []string{}}, want: RuleMaskedPaths},
		{name: "readonly paths override denied", hc: container.HostConfig{ReadonlyPaths: []string{}}, want: RuleMaskedPaths},
		{name: "volumes from denied", hc: container.HostConfig{VolumesFrom: []string{"other"}}, want: RuleVolumesFrom},
		{name: "named volume bind string allowed", hc: container.HostConfig{Binds: []string{"data:/var/lib/data"}}},
		{name: "docker socket bind denied", hc: container.HostConfig{Binds: []string{"/var/run/docker.sock:/var/run/docker.sock"}}, grant: binds, want: RuleBindSensitive},
		{name: "root bind denied", hc: container.HostConfig{Binds: []string{"/:/host"}}, want: RuleBindSensitive},
		{name: "etc bind denied", hc: container.HostConfig{Binds: []string{"/etc:/h:ro"}}, want: RuleBindSensitive},
		{name: "proc bind denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/proc", Target: "/p"}}}, want: RuleBindSensitive},
		{name: "sys bind denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/sys/fs", Target: "/p"}}}, want: RuleBindSensitive},
		{name: "root home bind denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/root/.ssh", Target: "/p"}}}, want: RuleBindSensitive},
		{name: "data dir bind denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/srv/levelrail-test-data/secrets", Target: "/p"}}}, want: RuleBindSensitive},
		{name: "data dir ancestor bind denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/srv", Target: "/p"}}}, want: RuleBindSensitive},
		{name: "traversal into etc denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/srv/app/../../etc", Target: "/p"}}}, want: RuleBindSensitive},
		{name: "undeclared bind denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/opt/data", Target: "/p"}}}, want: RuleBindUndeclared},
		{name: "relative bind denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "opt", Target: "/p"}}}, want: RuleBindUndeclared},
		{name: "declared bind allowed", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: declaredDir, Target: "/p"}}}, grant: binds},
		{name: "declared bind via string allowed", hc: container.HostConfig{Binds: []string{declaredDir + ":/p:ro"}}, grant: binds},
		{name: "named volume mount allowed", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "data", Target: "/p"}}}},
		{name: "tmpfs mount allowed", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeTmpfs, Target: "/tmp"}}}},
		{name: "volume with bind driver opts denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "x", Target: "/p", VolumeOptions: &mount.VolumeOptions{DriverConfig: &mount.Driver{Name: "local", Options: map[string]string{"type": "none", "o": "bind", "device": "/etc"}}}}}}, want: RuleVolumeBindDriverOpt},
		{name: "npipe mount denied", hc: container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeNamedPipe, Source: "x", Target: "/p"}}}, want: RuleMountType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testPolicy(t)
			p.AllowHostNetwork = tt.allowNet
			hc := tt.hc
			got := p.validateCreate(container.CreateRequest{Config: &container.Config{Image: "x"}, HostConfig: &hc}, tt.grant)
			switch {
			case tt.want == "" && len(got) > 0:
				t.Fatalf("want allowed, got %v", got)
			case tt.want != "" && (len(got) == 0 || got[0].Rule != tt.want):
				t.Fatalf("want rule %s, got %v", tt.want, rulesOf(got))
			}
		})
	}
}

func TestValidateCreateSymlinkToSensitive(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "innocent")
	if err := os.Symlink("/etc", link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	p := testPolicy(t)
	hc := container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: link, Target: "/p"}}}
	got := p.validateCreate(container.CreateRequest{HostConfig: &hc}, docker.CreateDeclaration{BindPaths: []string{link}})
	if len(got) == 0 || got[0].Rule != RuleBindSensitive {
		t.Fatalf("symlink to /etc: want %s, got %v", RuleBindSensitive, rulesOf(got))
	}
}

func TestValidateOtherBodies(t *testing.T) {
	tests := []struct {
		name string
		got  []Violation
		want string
	}{
		{name: "exec plain allowed", got: validateExec(container.ExecOptions{Cmd: []string{"sh"}})},
		{name: "exec privileged denied", got: validateExec(container.ExecOptions{Privileged: true}), want: RuleExecPrivileged},
		{name: "update resources allowed", got: validateUpdate(container.UpdateConfig{Resources: container.Resources{Memory: 1 << 20}})},
		{name: "update devices denied", got: validateUpdate(container.UpdateConfig{Resources: container.Resources{Devices: []container.DeviceMapping{{PathOnHost: "/dev/sda"}}}}), want: RuleDevices},
		{name: "update cgroup rules denied", got: validateUpdate(container.UpdateConfig{Resources: container.Resources{DeviceCgroupRules: []string{"a *:* rwm"}}}), want: RuleDeviceCgroupRules},
		{name: "volume plain allowed", got: validateVolume(volume.CreateOptions{Name: "v"})},
		{name: "volume nfs allowed", got: validateVolume(volume.CreateOptions{Name: "v", DriverOpts: map[string]string{"type": "nfs", "o": "addr=10.0.0.2,rw", "device": ":/export"}})},
		{name: "volume bind denied", got: validateVolume(volume.CreateOptions{Name: "v", DriverOpts: map[string]string{"type": "none", "O": "rbind,ro", "device": "/"}}), want: RuleVolumeBindDriverOpt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch {
			case tt.want == "" && len(tt.got) > 0:
				t.Fatalf("want allowed, got %v", tt.got)
			case tt.want != "" && (len(tt.got) == 0 || tt.got[0].Rule != tt.want):
				t.Fatalf("want %s, got %v", tt.want, rulesOf(tt.got))
			}
		})
	}
}

func TestGrantsLifecycle(t *testing.T) {
	g := NewGrants()
	release := g.DeclareCreate(docker.CreateDeclaration{Name: "/web", GPU: true})
	if !g.lookup("web").GPU {
		t.Fatal("declared grant not visible")
	}
	release()
	if g.lookup("web").GPU {
		t.Fatal("grant survived release")
	}
	g.DeclareCreate(docker.CreateDeclaration{GPU: true})
	if g.lookup("").GPU {
		t.Fatal("unnamed create must not be granted")
	}
}
