package gpu

import (
	"context"
	"errors"
	"testing"
)

type fakeRunner struct {
	out string
	err error
}

func (f fakeRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return []byte(f.out), f.err
}

type fakeRuntimes struct {
	names []string
	err   error
}

func (f fakeRuntimes) RuntimeNames(context.Context) ([]string, error) { return f.names, f.err }

const twoGPUs = "0, GPU-aaa, NVIDIA A100-SXM4-40GB, 40960, 1024, 7, 550.54.15\n1, GPU-bbb, NVIDIA A100-SXM4-40GB, 40960, 0, [N/A], 550.54.15\n"

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		runner      fakeRunner
		runtimes    fakeRuntimes
		wantPresent bool
		wantCount   int
		wantRuntime bool
		wantHint    bool
		wantVRAM    int64
	}{
		{name: "no nvidia-smi", runner: fakeRunner{err: errors.New("not found")}},
		{name: "empty output", runner: fakeRunner{out: ""}},
		{name: "garbage output", runner: fakeRunner{out: "NVIDIA-SMI has failed"}},
		{name: "gpus with runtime", runner: fakeRunner{out: twoGPUs}, runtimes: fakeRuntimes{names: []string{"runc", "nvidia"}}, wantPresent: true, wantCount: 2, wantRuntime: true, wantVRAM: 81920},
		{name: "gpus without runtime", runner: fakeRunner{out: twoGPUs}, runtimes: fakeRuntimes{names: []string{"runc"}}, wantPresent: true, wantCount: 2, wantHint: true, wantVRAM: 81920},
		{name: "docker info fails", runner: fakeRunner{out: twoGPUs}, runtimes: fakeRuntimes{err: errors.New("boom")}, wantPresent: true, wantCount: 2, wantHint: true, wantVRAM: 81920},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect(context.Background(), tt.runner, tt.runtimes)
			if got.Present != tt.wantPresent || got.Count() != tt.wantCount {
				t.Fatalf("Present/Count = %v/%d, want %v/%d", got.Present, got.Count(), tt.wantPresent, tt.wantCount)
			}
			if got.RuntimeInstalled != tt.wantRuntime || got.Usable() != tt.wantRuntime {
				t.Errorf("RuntimeInstalled = %v, want %v", got.RuntimeInstalled, tt.wantRuntime)
			}
			if (got.Hint() != "") != tt.wantHint {
				t.Errorf("Hint() = %q, wantHint %v", got.Hint(), tt.wantHint)
			}
			if got.TotalVRAMMiB() != tt.wantVRAM {
				t.Errorf("TotalVRAMMiB = %d, want %d", got.TotalVRAMMiB(), tt.wantVRAM)
			}
		})
	}
}

func TestParseSMI(t *testing.T) {
	driver, devs, err := ParseSMI(twoGPUs)
	if err != nil {
		t.Fatalf("ParseSMI: %v", err)
	}
	if driver != "550.54.15" || len(devs) != 2 {
		t.Fatalf("driver=%q devs=%d", driver, len(devs))
	}
	if devs[0].UUID != "GPU-aaa" || devs[0].Name != "NVIDIA A100-SXM4-40GB" || devs[0].VRAMUsedMiB != 1024 || devs[0].UtilizationPercent != 7 {
		t.Errorf("device 0 = %+v", devs[0])
	}
	if devs[1].UtilizationPercent != 0 {
		t.Errorf("N/A utilization = %d, want 0", devs[1].UtilizationPercent)
	}
	if _, _, err := ParseSMI("x, y, z, notnum, 1, 2, 3"); err == nil {
		t.Error("ParseSMI with non-numeric memory: want error")
	}
}
