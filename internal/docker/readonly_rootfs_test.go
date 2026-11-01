package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func rorSpec(label string) ContainerSpec {
	s := ContainerSpec{Name: "a"}
	if label != "" {
		s.Labels = map[string]string{ReadOnlyRootfsLabelKey: label}
	}
	return s
}

func TestApplyReadOnlyRootfs(t *testing.T) {
	withVol := rorSpec("true")
	withVol.Volumes = []VolumeMount{{Name: "v", ContainerPath: "/tmp"}}
	withFiles := rorSpec("")
	withFiles.Files = []FileMount{{Path: "/etc/x"}}
	tests := []struct {
		name      string
		cfg       HardeningConfig
		spec      ContainerSpec
		wantRO    bool
		wantTmpfs []string
	}{
		{"default off", HardeningConfig{}, rorSpec(""), false, nil},
		{"per-app true", HardeningConfig{}, rorSpec("true"), true, []string{"/run", "/tmp"}},
		{"global true inherited", HardeningConfig{ReadOnlyRootfs: true}, rorSpec(""), true, []string{"/run", "/tmp"}},
		{"per-app false beats global true", HardeningConfig{ReadOnlyRootfs: true}, rorSpec("false"), false, nil},
		{"volume on /tmp keeps the volume", HardeningConfig{}, withVol, true, []string{"/run"}},
		{"global default skips services with files", HardeningConfig{ReadOnlyRootfs: true}, withFiles, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := &container.HostConfig{}
			tt.cfg.applyReadOnlyRootfs(hc, tt.spec)
			if hc.ReadonlyRootfs != tt.wantRO || len(hc.Tmpfs) != len(tt.wantTmpfs) {
				t.Fatalf("readonly=%v tmpfs=%v, want readonly=%v tmpfs=%v", hc.ReadonlyRootfs, hc.Tmpfs, tt.wantRO, tt.wantTmpfs)
			}
			for _, p := range tt.wantTmpfs {
				if hc.Tmpfs[p] == "" {
					t.Fatalf("missing tmpfs %s in %v", p, hc.Tmpfs)
				}
			}
		})
	}
}

func TestHardeningFromEnv_ReadOnlyRootfs(t *testing.T) {
	t.Setenv("APP_CONTAINER_READONLY_ROOTFS", "true")
	t.Setenv("APP_CONTAINER_READONLY_TMPFS_MB", "32")
	cfg, err := HardeningFromEnv(RuntimeInfo{})
	if err != nil || !cfg.ReadOnlyRootfs || cfg.ReadOnlyTmpfsMB != 32 {
		t.Fatalf("cfg = %+v err = %v", cfg, err)
	}
	t.Setenv("APP_CONTAINER_READONLY_ROOTFS", "maybe")
	if _, err := HardeningFromEnv(RuntimeInfo{}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}
