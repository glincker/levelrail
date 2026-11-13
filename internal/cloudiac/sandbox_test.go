package cloudiac

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

const canaryCredential = "lr-canary-header-3f9a1c7e\nlr-canary-3f9a1c7e2b8d4\nlr-canary-footer-9d2e" //nolint:gosec // leak canary, not a real key

func sandboxInput(t *testing.T) SandboxInput {
	t.Helper()
	np, err := NewNetworkPolicy("lr-cloud-egress", []string{"10.20.0.5", "fd12::/64"})
	if err != nil {
		t.Fatal(err)
	}
	return SandboxInput{
		Runtime:    "docker",
		Image:      "cgr.example/static@sha256:" + strings.Repeat("a", 64),
		User:       "65532:65532",
		Name:       "lr-step-1",
		Step:       StepPlan,
		ModuleDir:  "/var/lib/levelrail/cloudiac/runs/r1/module",
		TofuBinary: "/var/lib/levelrail/cloudiac/tofu/1.13.2/linux_amd64/tofu",
		PluginDir:  "/var/lib/levelrail/cloudiac/plugins",
		Credential: CredentialFile{HostPath: "/run/user/1000/lr-cred-r1/credential", EnvName: "OCI_CLI_KEY_FILE"},
		Network:    np,
		Limits:     SandboxLimits{MemoryMB: 1024, CPUs: 1, Pids: 256, WorkTmpMB: 512, StateTmpMB: 64},
		Env:        map[string]string{"OCI_REGION": "eu-frankfurt-1"},
	}
}

func TestSandboxSpecFlags(t *testing.T) {
	in := sandboxInput(t)
	spec, err := BuildSandboxSpec(in)
	if err != nil {
		t.Fatal(err)
	}
	argv := spec.Argv()
	want := []string{
		"docker", "run", "--rm", "--pull=never", "--name=lr-step-1", "--read-only",
		"--cap-drop=ALL", "--security-opt=no-new-privileges", "--user=65532:65532",
		"--pids-limit=256", "--memory=1024m", "--memory-swap=1024m", "--cpus=1",
		"--ipc=private", "--hostname=lr-cloud-step", "--network=lr-cloud-egress",
		"--mount=type=bind,source=/var/lib/levelrail/cloudiac/runs/r1/module,target=/module,readonly",
		"--mount=type=bind,source=/var/lib/levelrail/cloudiac/tofu/1.13.2/linux_amd64/tofu,target=/opt/tofu/tofu,readonly",
		"--mount=type=bind,source=/var/lib/levelrail/cloudiac/plugins,target=/opt/tofu/plugins,readonly",
		"--mount=type=bind,source=/run/user/1000/lr-cred-r1/credential,target=/run/lr-cred/credential,readonly",
		"--tmpfs=/work:rw,nosuid,nodev,size=512m,uid=65532,gid=65532,mode=0700",
		"--tmpfs=/state:rw,noexec,nosuid,nodev,size=64m,uid=65532,gid=65532,mode=0700",
		"--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=64m,uid=65532,gid=65532,mode=0700",
		"--env=HOME=/work", "--env=TF_DATA_DIR=/work/.tofu", "--env=TF_IN_AUTOMATION=1", "--env=TF_INPUT=0",
		"--env=OCI_CLI_KEY_FILE=/run/lr-cred/credential", "--env=TF_PLUGIN_CACHE_DIR=/opt/tofu/plugins",
		"--env=OCI_REGION=eu-frankfurt-1",
		"--workdir=/work", "--entrypoint=/opt/tofu/tofu", in.Image,
		"-chdir=/module", "plan", "-input=false", "-no-color", "-lock=false", "-out=/state/plan.tfplan",
	}
	if !slices.Equal(argv, want) {
		t.Fatalf("argv mismatch\n got: %q\nwant: %q", argv, want)
	}
	joined := strings.Join(argv, " ")
	for _, banned := range []string{"--privileged", "docker.sock", "--network=host", "--cap-add", "-v ", "--volume"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("argv contains %q", banned)
		}
	}
	for _, line := range strings.Split(canaryCredential, "\n") {
		if strings.Contains(joined, line) {
			t.Fatal("credential material leaked into argv")
		}
	}
}

func TestSandboxStepArgs(t *testing.T) {
	apply, _ := StepArgs(StepApply)
	if !slices.Equal(apply, []string{"-chdir=/module", "apply", "-input=false", "-no-color", "-lock=false", "/state/plan.tfplan"}) {
		t.Fatalf("apply must run the saved plan only: %q", apply)
	}
	destroy, _ := StepArgs(StepDestroy)
	if !slices.Contains(destroy, "-destroy") || slices.Contains(destroy, "-auto-approve") {
		t.Fatalf("destroy must be a plan: %q", destroy)
	}
	if _, err := StepArgs("init; rm"); !errors.Is(err, ErrSandboxSpec) {
		t.Fatal("unknown step must be refused")
	}
}

func TestSandboxRefusesUnsafeInput(t *testing.T) {
	cases := map[string]func(*SandboxInput){
		"root user":         func(in *SandboxInput) { in.User = "0:0" },
		"named user":        func(in *SandboxInput) { in.User = "root" },
		"tag not digest":    func(in *SandboxInput) { in.Image = "alpine:3" },
		"other runtime":     func(in *SandboxInput) { in.Runtime = "sh" },
		"docker socket":     func(in *SandboxInput) { in.ModuleDir = "/var/run/docker.sock" },
		"podman socket":     func(in *SandboxInput) { in.PluginDir = "/run/user/1000/podman/podman.sock" },
		"host root":         func(in *SandboxInput) { in.ModuleDir = "/" },
		"relative mount":    func(in *SandboxInput) { in.ModuleDir = "module" },
		"dotdot mount":      func(in *SandboxInput) { in.ModuleDir = "/var/lib/../../etc" },
		"etc mount":         func(in *SandboxInput) { in.TofuBinary = "/etc/shadow" },
		"no memory limit":   func(in *SandboxInput) { in.Limits.MemoryMB = 0 },
		"no pids limit":     func(in *SandboxInput) { in.Limits.Pids = 0 },
		"no cpu limit":      func(in *SandboxInput) { in.Limits.CPUs = 0 },
		"hand-made network": func(in *SandboxInput) { in.Network = NetworkPolicy{Network: "host"} },
		"secret env":        func(in *SandboxInput) { in.Env = map[string]string{"AWS_SECRET_ACCESS_KEY": "x"} },
		"tf env override":   func(in *SandboxInput) { in.Env = map[string]string{"TF_CLI_ARGS": "-auto-approve"} },
		"bad cred env":      func(in *SandboxInput) { in.Credential.EnvName = "a=b" },
		"no credential":     func(in *SandboxInput) { in.Credential.HostPath = "" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			in := sandboxInput(t)
			mut(&in)
			if _, err := BuildSandboxSpec(in); !errors.Is(err, ErrSandboxSpec) {
				t.Fatalf("want refusal, got %v", err)
			}
		})
	}
}

func TestNetworkPolicyDenies(t *testing.T) {
	for _, n := range []string{"", "host", "bridge", "none", "container:abc"} {
		if _, err := NewNetworkPolicy(n, []string{"10.0.0.1"}); !errors.Is(err, ErrSandboxSpec) {
			t.Fatalf("network %q must be refused", n)
		}
	}
	if _, err := NewNetworkPolicy("lr-net", nil); !errors.Is(err, ErrSandboxSpec) {
		t.Fatal("control plane addresses are required")
	}
	np, err := NewNetworkPolicy("lr-net", []string{"10.20.0.5", "fd12::/64"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(np.RunFlags(), []string{"--network=lr-net"}) {
		t.Fatalf("flags %q", np.RunFlags())
	}
	for _, a := range []string{"127.0.0.1", "169.254.169.254", "::1", "fe80::1", "fd00:ec2::254", "10.20.0.5", "fd12::9", "::ffff:127.0.0.1", "not-an-ip"} {
		if !np.Denies(a) {
			t.Fatalf("%s must be denied", a)
		}
	}
	for _, a := range []string{"34.120.0.1", "2600:1f18::1"} {
		if np.Denies(a) {
			t.Fatalf("cloud API address %s must stay reachable", a)
		}
	}
}

func TestRedactLogMasksCredential(t *testing.T) {
	log := "init ok\nError: bad key " + canaryCredential + "\nline lr-canary-3f9a1c7e2b8d4 again\nplan done"
	out := RedactLog(log, canaryCredential, "")
	for _, line := range strings.Split(canaryCredential, "\n") {
		if strings.Contains(out, line) {
			t.Fatalf("redacted log still holds %q", line)
		}
	}
	if !strings.Contains(out, "plan done") || !strings.Contains(out, redacted) {
		t.Fatalf("redaction removed too much or nothing: %q", out)
	}
}
