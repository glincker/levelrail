package docker

import (
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

func TestHardeningApply_Matrix(t *testing.T) {
	tests := []struct {
		name        string
		cfg         HardeningConfig
		spec        ContainerSpec
		wantHarden  bool
		wantCapAdd  []string
		wantPids    int64
		wantSecOpts []string
	}{
		{name: "zero value applies nothing", cfg: HardeningConfig{}, spec: ContainerSpec{Name: "a"}},
		{name: "off applies nothing", cfg: HardeningConfig{Mode: HardeningOff, PidsLimit: 10}, spec: ContainerSpec{Name: "a"}},
		{name: "warn applies nothing", cfg: HardeningConfig{Mode: HardeningWarn, PidsLimit: 10}, spec: ContainerSpec{Name: "a"}},
		{
			name: "enforce applies minimal set", cfg: HardeningConfig{Mode: HardeningEnforce, PidsLimit: 100},
			spec: ContainerSpec{Name: "a"}, wantHarden: true,
			wantCapAdd: []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL", "NET_BIND_SERVICE", "SETGID", "SETUID"},
			wantPids:   100, wantSecOpts: []string{"no-new-privileges"},
		},
		{
			name: "enforce keeps the egress sidecar NET_ADMIN", cfg: HardeningConfig{Mode: HardeningEnforce},
			spec: ContainerSpec{Name: "egress", CapAdd: []string{"NET_ADMIN"}}, wantHarden: true,
			wantCapAdd:  []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL", "NET_ADMIN", "NET_BIND_SERVICE", "SETGID", "SETUID"},
			wantSecOpts: []string{"no-new-privileges"},
		},
		{
			name: "enforce merges operator extra caps without duplicates",
			cfg:  HardeningConfig{Mode: HardeningEnforce, ExtraCaps: []string{"SYS_CHROOT", "CHOWN"}},
			spec: ContainerSpec{Name: "a"}, wantHarden: true,
			wantCapAdd:  []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL", "NET_BIND_SERVICE", "SETGID", "SETUID", "SYS_CHROOT"},
			wantSecOpts: []string{"no-new-privileges"},
		},
		{
			name: "pids limit 0 disables only the pid cap", cfg: HardeningConfig{Mode: HardeningEnforce, PidsLimit: 0},
			spec: ContainerSpec{Name: "a"}, wantHarden: true,
			wantCapAdd:  []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL", "NET_BIND_SERVICE", "SETGID", "SETUID"},
			wantSecOpts: []string{"no-new-privileges"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := buildHostConfig(tt.spec, nat.PortMap{})
			before := hc.CapAdd
			tt.cfg.apply(hc, tt.spec)
			if !tt.wantHarden {
				if len(hc.CapDrop) != 0 || len(hc.SecurityOpt) != 0 || hc.PidsLimit != nil || !reflect.DeepEqual(hc.CapAdd, before) {
					t.Fatalf("expected untouched HostConfig, got %+v", hc)
				}
				return
			}
			if !reflect.DeepEqual([]string(hc.CapDrop), []string{"ALL"}) {
				t.Errorf("CapDrop = %v, want [ALL]", hc.CapDrop)
			}
			if !reflect.DeepEqual([]string(hc.CapAdd), tt.wantCapAdd) {
				t.Errorf("CapAdd = %v, want %v", hc.CapAdd, tt.wantCapAdd)
			}
			if !reflect.DeepEqual(hc.SecurityOpt, tt.wantSecOpts) {
				t.Errorf("SecurityOpt = %v, want %v", hc.SecurityOpt, tt.wantSecOpts)
			}
			switch {
			case tt.wantPids == 0 && hc.PidsLimit != nil:
				t.Errorf("PidsLimit = %d, want nil", *hc.PidsLimit)
			case tt.wantPids != 0 && (hc.PidsLimit == nil || *hc.PidsLimit != tt.wantPids):
				t.Errorf("PidsLimit = %v, want %d", hc.PidsLimit, tt.wantPids)
			}
			if hc.ReadonlyRootfs || hc.Privileged {
				t.Error("read-only rootfs and privileged must never be set by defaults")
			}
		})
	}
}

func TestHardeningApply_DoesNotOverrideExistingPidsLimit(t *testing.T) {
	existing := int64(7)
	hc := &container.HostConfig{Resources: container.Resources{PidsLimit: &existing}}
	HardeningConfig{Mode: HardeningEnforce, PidsLimit: 100}.apply(hc, ContainerSpec{})
	if hc.PidsLimit == nil || *hc.PidsLimit != 7 {
		t.Fatalf("PidsLimit = %v, want 7", hc.PidsLimit)
	}
}

func TestHardeningFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		pids    string
		caps    string
		want    HardeningConfig
		wantErr bool
	}{
		{name: "defaults to warn", want: HardeningConfig{Mode: HardeningWarn, PidsLimit: DefaultPidsLimit}},
		{name: "enforce with overrides", mode: "Enforce", pids: "50", caps: "cap_sys_chroot, net_raw",
			want: HardeningConfig{Mode: HardeningEnforce, PidsLimit: 50, ExtraCaps: []string{"SYS_CHROOT", "NET_RAW"}}},
		{name: "off", mode: "off", want: HardeningConfig{Mode: HardeningOff, PidsLimit: DefaultPidsLimit}},
		{name: "bad mode falls back to warn", mode: "strict", wantErr: true, want: HardeningConfig{Mode: HardeningWarn, PidsLimit: DefaultPidsLimit}},
		{name: "bad pids keeps default", mode: "enforce", pids: "lots", wantErr: true, want: HardeningConfig{Mode: HardeningEnforce, PidsLimit: DefaultPidsLimit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envHardening, tt.mode)
			t.Setenv(envHardeningPids, tt.pids)
			t.Setenv(envHardeningCaps, tt.caps)
			got, err := HardeningFromEnv()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHardeningReport(t *testing.T) {
	if r := (HardeningConfig{Mode: HardeningOff}).Report(); r.Applied || r.CapDrop != nil {
		t.Errorf("off report = %+v", r)
	}
	warn := HardeningConfig{Mode: HardeningWarn, PidsLimit: 9}.Report()
	if warn.Applied || !warn.NoNewPrivileges || warn.PidsLimit != 9 || len(warn.CapAdd) != len(MinimalCapabilities) {
		t.Errorf("warn report = %+v", warn)
	}
	if !(HardeningConfig{Mode: HardeningEnforce}).Report().Applied {
		t.Error("enforce report must be applied")
	}
}
