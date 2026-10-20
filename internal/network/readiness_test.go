package network

import "testing"

type stubProbe struct {
	os         string
	module     bool
	privileged bool
}

func (p stubProbe) OS() string               { return p.os }
func (p stubProbe) KernelModuleLoaded() bool { return p.module }
func (p stubProbe) Privileged() bool         { return p.privileged }

func TestReadinessChecks(t *testing.T) {
	base := func() ReadinessEnv {
		return ReadinessEnv{
			Probe:            stubProbe{os: "linux", module: true, privileged: true},
			TunDevicePresent: func() bool { return true },
			UDPPortFree:      func(int) (bool, error) { return true, nil },
			UFWActive:        func() bool { return false },
			AdvertiseHost:    "203.0.113.5",
		}
	}
	tests := []struct {
		name   string
		mutate func(*ReadinessEnv)
		id     string
		want   string
	}{
		{"all good", func(*ReadinessEnv) {}, CheckWireGuard, CheckPass},
		{"unprivileged", func(e *ReadinessEnv) { e.Probe = stubProbe{os: "linux", privileged: false} }, CheckWireGuard, CheckFail},
		{"no module and no tun", func(e *ReadinessEnv) {
			e.Probe = stubProbe{os: "linux", privileged: true}
			e.TunDevicePresent = func() bool { return false }
		}, CheckWireGuard, CheckFail},
		{"userspace with tun", func(e *ReadinessEnv) { e.Probe = stubProbe{os: "linux", privileged: true} }, CheckWireGuard, CheckPass},
		{"port in use", func(e *ReadinessEnv) { e.UDPPortFree = func(int) (bool, error) { return false, nil } }, CheckUDPPort, CheckFail},
		{"port in use but mesh running", func(e *ReadinessEnv) {
			e.UDPPortFree = func(int) (bool, error) { return false, nil }
			e.MeshRunning = true
		}, CheckUDPPort, CheckPass},
		{"ufw active", func(e *ReadinessEnv) { e.UFWActive = func() bool { return true } }, CheckFirewall, CheckWarn},
		{"loopback advertise host", func(e *ReadinessEnv) { e.AdvertiseHost = "127.0.0.1" }, CheckPublicAddress, CheckWarn},
		{"empty advertise host", func(e *ReadinessEnv) { e.AdvertiseHost = "" }, CheckPublicAddress, CheckWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := base()
			tt.mutate(&e)
			for _, c := range ReadinessChecks(e) {
				if c.ID == tt.id {
					if c.Status != tt.want {
						t.Fatalf("%s = %s (%s), want %s", c.ID, c.Status, c.Detail, tt.want)
					}
					if c.Status != CheckPass && c.Hint == "" {
						t.Fatalf("%s has no fix hint", c.ID)
					}
					return
				}
			}
			t.Fatalf("no check %s", tt.id)
		})
	}
}

func TestResolveEnabled(t *testing.T) {
	env := func(v string, set bool) func(string) (string, bool) {
		return func(string) (string, bool) { return v, set }
	}
	tests := []struct {
		name         string
		lookup       func(string) (string, bool)
		saved, isSet bool
		want         bool
		src          string
	}{
		{"default off", env("", false), false, false, false, SettingSourceDefault},
		{"saved on", env("", false), true, true, true, SettingSourceSettings},
		{"env on beats saved off", env("1", true), false, true, true, SettingSourceEnv},
		{"env off beats saved on", env("0", true), true, true, false, SettingSourceEnv},
		{"blank env is unset", env("  ", true), true, true, true, SettingSourceSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, src := ResolveEnabled(tt.lookup, tt.saved, tt.isSet)
			if got != tt.want || src != tt.src {
				t.Fatalf("got %v %s, want %v %s", got, src, tt.want, tt.src)
			}
		})
	}
}
