package gpu

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type cmdRunner map[string]string

func (f cmdRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if out, ok := f[name]; ok {
		return []byte(out), nil
	}
	return nil, errors.New("not found: " + name)
}

type fakeDocker struct {
	runtimes []string
	cdi      []string
}

func (f fakeDocker) RuntimeNames(context.Context) ([]string, error) { return f.runtimes, nil }
func (f fakeDocker) CDIDevices(context.Context) ([]string, error)   { return f.cdi, nil }

const smiOne = "0, GPU-a, RTX 4090, 24564, 1200, 3, 550.54.03\n"

func findingByCode(t *testing.T, h HostDiagnosis, code string) Finding {
	t.Helper()
	for _, f := range h.Findings {
		if f.Code == code {
			return f
		}
	}
	t.Fatalf("no %q finding in %+v", code, h.Findings)
	return Finding{}
}

func TestDiagnoseHost(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "nvidia.yaml")
	if err := os.WriteFile(spec, []byte("kind: nvidia.com/gpu\ndevices: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.yaml")
	if err := os.WriteFile(other, []byte("kind: vendor.example/thing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withSpec := func(string) []string { return []string{spec, other, filepath.Join(dir, "notes.txt")} }
	noSpec := func(string) []string { return nil }
	ok := cmdRunner{"nvidia-smi": smiOne, "nvidia-ctk": "NVIDIA Container Toolkit CLI version 1.17.8\ncommit: abc\n"}

	tests := []struct {
		name       string
		r          cmdRunner
		d          fakeDocker
		glob       func(string) []string
		wantAttach string
		wantCodes  map[string]string
	}{
		{
			name: "cdi devices discovered", r: ok, d: fakeDocker{runtimes: []string{"runc", "nvidia"}, cdi: []string{"nvidia.com/gpu=all"}}, glob: withSpec,
			wantAttach: AttachCDI, wantCodes: map[string]string{"driver": StatusOK, "toolkit": StatusOK, "cdi": StatusOK, "attach": StatusOK, "runtime": StatusOK},
		},
		{
			name: "spec present but docker lists no devices falls back to legacy", r: ok, d: fakeDocker{runtimes: []string{"nvidia"}}, glob: withSpec,
			wantAttach: AttachLegacy, wantCodes: map[string]string{"cdi": StatusWarn, "attach": StatusOK, "runtime": StatusOK},
		},
		{
			name: "no spec and legacy runtime is fine", r: ok, d: fakeDocker{runtimes: []string{"nvidia"}}, glob: noSpec,
			wantAttach: AttachLegacy, wantCodes: map[string]string{"cdi": StatusOK, "attach": StatusOK},
		},
		{
			name: "neither path works", r: ok, d: fakeDocker{runtimes: []string{"runc"}}, glob: noSpec,
			wantAttach: AttachNone, wantCodes: map[string]string{"attach": StatusFail, "runtime": StatusWarn},
		},
		{
			name: "toolkit missing is a warning", r: cmdRunner{"nvidia-smi": smiOne}, d: fakeDocker{runtimes: []string{"nvidia"}}, glob: noSpec,
			wantAttach: AttachLegacy, wantCodes: map[string]string{"toolkit": StatusWarn, "attach": StatusOK},
		},
		{
			name: "no driver", r: cmdRunner{}, d: fakeDocker{}, glob: noSpec,
			wantAttach: AttachNone, wantCodes: map[string]string{"driver": StatusFail},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := DiagnoseHost(context.Background(), tc.r, tc.d, tc.glob)
			if h.Attach != tc.wantAttach {
				t.Errorf("attach = %s, want %s", h.Attach, tc.wantAttach)
			}
			for code, want := range tc.wantCodes {
				if got := findingByCode(t, h, code).Status; got != want {
					t.Errorf("%s = %s, want %s", code, got, want)
				}
			}
		})
	}
}

func TestDiagnoseHostReportsToolkitVersionAndOnlyNvidiaSpecs(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "nvidia.yaml")
	if err := os.WriteFile(spec, []byte("kind: nvidia.com/gpu\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.yaml")
	if err := os.WriteFile(other, []byte("kind: vendor.example/thing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := cmdRunner{"nvidia-smi": smiOne, "nvidia-ctk": "NVIDIA Container Toolkit CLI version 1.17.8\ncommit: abc\n"}
	h := DiagnoseHost(context.Background(), r, fakeDocker{runtimes: []string{"nvidia"}}, func(string) []string { return []string{spec, other} })
	if h.ToolkitVersion != "NVIDIA Container Toolkit CLI version 1.17.8" {
		t.Errorf("toolkit version = %q", h.ToolkitVersion)
	}
	if len(h.CDISpecFiles) != 1 || h.CDISpecFiles[0] != spec {
		t.Errorf("spec files = %v, want only the nvidia one", h.CDISpecFiles)
	}
}

func TestDiagnoseHostWarnsOnFullGPUMemory(t *testing.T) {
	t.Setenv(envVRAMWarnPercent, "80")
	r := cmdRunner{"nvidia-smi": "0, GPU-a, RTX 4090, 24000, 20000, 90, 550.54.03\n1, GPU-b, RTX 4090, 24000, 100, 0, 550.54.03\n"}
	h := DiagnoseHost(context.Background(), r, fakeDocker{runtimes: []string{"nvidia"}}, func(string) []string { return nil })
	if got := findingByCode(t, h, "gpu0"); got.Status != StatusWarn {
		t.Errorf("gpu0 = %+v, want warn at the 80%% threshold", got)
	}
	if got := findingByCode(t, h, "gpu1"); got.Status != StatusOK {
		t.Errorf("gpu1 = %+v, want ok", got)
	}
}

func TestDetectCountsCDIAsAUsableRuntime(t *testing.T) {
	r := cmdRunner{"nvidia-smi": smiOne}
	if got := Detect(context.Background(), r, fakeDocker{runtimes: []string{"runc"}, cdi: []string{"nvidia.com/gpu=all"}}); !got.RuntimeInstalled || got.Hint() != "" {
		t.Errorf("a daemon listing CDI devices needs no nvidia runtime: %+v", got)
	}
	if got := Detect(context.Background(), r, fakeDocker{runtimes: []string{"runc"}}); got.RuntimeInstalled || got.Hint() == "" {
		t.Errorf("no runtime and no CDI must still hint: %+v", got)
	}
	if got := Detect(context.Background(), r, fakeDocker{runtimes: []string{"nvidia"}}); !got.RuntimeInstalled {
		t.Errorf("the nvidia runtime alone is enough: %+v", got)
	}
}
