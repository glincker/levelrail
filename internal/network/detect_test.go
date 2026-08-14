package network

import (
	"strings"
	"testing"
)

// fakeProbe supplies Detect's evidence directly, which is the whole
// reason Probe is an interface: none of these branches is reachable in a
// test otherwise, and every one of them is a different thing an operator
// has to be told.
type fakeProbe struct {
	os         string
	moduleLoad bool
	privileged bool
}

func (p fakeProbe) OS() string               { return p.os }
func (p fakeProbe) KernelModuleLoaded() bool { return p.moduleLoad }
func (p fakeProbe) Privileged() bool         { return p.privileged }

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		probe       fakeProbe
		wantBackend Backend
		wantReason  string
	}{
		{
			name:        "linux with the module loaded takes the fast path",
			probe:       fakeProbe{os: "linux", moduleLoad: true, privileged: true},
			wantBackend: BackendKernel,
			wantReason:  "in-kernel wireguard module is loaded",
		},
		{
			name:        "linux without the module falls back to userspace",
			probe:       fakeProbe{os: "linux", moduleLoad: false, privileged: true},
			wantBackend: BackendUserspace,
			wantReason:  "not loaded",
		},
		{
			name:        "darwin has no kernel module to find",
			probe:       fakeProbe{os: "darwin", privileged: true},
			wantBackend: BackendUserspace,
			wantReason:  "darwin",
		},
		{
			name: "a loaded module on a non linux host is still userspace",
			// Not reachable through SystemProbe, which returns false off
			// Linux, but Detect must not depend on that: a Probe is an
			// interface and the rule is "kernel path is a Linux thing",
			// enforced here rather than assumed upstream.
			probe:       fakeProbe{os: "freebsd", moduleLoad: true, privileged: true},
			wantBackend: BackendUserspace,
			wantReason:  "freebsd",
		},
		{
			name:        "unprivileged disables the mesh entirely",
			probe:       fakeProbe{os: "linux", moduleLoad: true, privileged: false},
			wantBackend: BackendDisabled,
			wantReason:  "insufficient privileges",
		},
		{
			name:        "unprivileged without a module is also disabled",
			probe:       fakeProbe{os: "linux", privileged: false},
			wantBackend: BackendDisabled,
			wantReason:  "insufficient privileges",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Detect(tc.probe)
			if got.Backend != tc.wantBackend {
				t.Errorf("Backend = %q, want %q", got.Backend, tc.wantBackend)
			}
			if !strings.Contains(got.Reason, tc.wantReason) {
				t.Errorf("Reason = %q, want it to mention %q", got.Reason, tc.wantReason)
			}
			if got.Reason == "" {
				t.Error("Reason is empty: a backend with no explanation is a mystery, not a decision")
			}
		})
	}
}

func TestSystemProbe_DoesNotPanic(t *testing.T) {
	// The real probe reads this machine's own state, so the only
	// assertion available is that it answers at all and that its answers
	// are internally consistent with the platform running the test.
	p := SystemProbe{}
	if p.OS() == "" {
		t.Error("OS() is empty")
	}
	if p.OS() != "linux" && p.KernelModuleLoaded() {
		t.Error("KernelModuleLoaded() is true off Linux, where there is no such module")
	}
	_ = p.Privileged()
}

func TestInterfaceName(t *testing.T) {
	tests := []struct {
		name      string
		shortName string
		want      string
	}{
		{name: "simple", shortName: "acme", want: "acme0"},
		{name: "mixed case is lowercased", shortName: "AcMe", want: "acme0"},
		{name: "punctuation is stripped", shortName: "a-c.m e", want: "acme0"},
		{name: "digits are kept", shortName: "acme2", want: "acme20"},
		{name: "empty falls back to a non branded name", shortName: "", want: "wg0"},
		{name: "all punctuation falls back too", shortName: "-.-", want: "wg0"},
		{
			// Linux caps interface names at 15 characters (IFNAMSIZ-1)
			// and fails at device creation with an error that says
			// nothing about length.
			name:      "long names are truncated to the kernel limit",
			shortName: "averyverylongbrandnameindeed",
			want:      "averyverylongb0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := interfaceName(tc.shortName)
			if got != tc.want {
				t.Errorf("interfaceName(%q) = %q, want %q", tc.shortName, got, tc.want)
			}
			if len(got) > 15 {
				t.Errorf("interfaceName(%q) = %q, %d characters, over the 15 character kernel limit",
					tc.shortName, got, len(got))
			}
		})
	}
}

// TestInterfaceName_NoProductNameInSource is the "no product name string
// in source" rule applied to this package specifically: the interface
// name has to come from the caller's brand short name, never from a
// constant here.
func TestInterfaceName_FollowsTheShortName(t *testing.T) {
	first := interfaceName("brandone")
	second := interfaceName("brandtwo")
	if first == second {
		t.Fatal("interface name does not follow the brand short name: a rebrand would not change it")
	}
}
