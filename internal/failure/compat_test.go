package failure

import "testing"

func TestClassifyCompat(t *testing.T) {
	cases := map[string]string{
		`service "agent": mounting the Docker socket (/var/run/docker.sock) is never allowed without a docker_api grant`: CompatSocketRefused,
		`capability class is disabled: devices (elevated tier) is off in runtime settings`:                               CompatDeviceMissing,
		`capability class is disabled: net_admin (elevated tier) is off in runtime settings`:                             CompatCapRefused,
		`template "x" has no linux/arm64 build (verified: linux/amd64).`:                                                 CompatArchMismatch,
		`exec /entrypoint.sh: exec format error`:                                                                         CompatArchMismatch,
		`Bind for 0.0.0.0:53 failed: port is already allocated`:                                                          CompatPortInUse,
		`modprobe: FATAL: Module wireguard not found in /lib/modules/6.1`:                                                CompatKernelModuleMissing,
		`template "ollama" needs an NVIDIA GPU and this node reports none.`:                                              CompatDeviceMissing,
	}
	for text, want := range cases {
		got, ok := ClassifyCompat(text)
		if !ok || got.ID != want {
			t.Errorf("ClassifyCompat(%q) = %q, %v; want %q", text, got.ID, ok, want)
		}
	}
	if _, ok := ClassifyCompat("an app named x already exists"); ok {
		t.Error("unrelated refusal must not classify")
	}
	for _, c := range CompatClasses() {
		if c.ID == "" || c.Cause == "" || c.Fix == "" {
			t.Errorf("class %q is missing fields", c.ID)
		}
	}
}
