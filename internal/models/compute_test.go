package models

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func testPolicy() CPUPolicy {
	return CPUPolicy{DefaultMemoryPercent: 25, MaxCPUPercent: 75, MaxMemoryPercent: 60, FreeRAMFloorMiB: 1024, CPUShares: 128, OomScoreAdj: 800,
		PidsLimit: 256, DefaultContext: 4096, CachePercent: 5, CacheMaxMiB: 512, MmapHeadroomMiB: 1024, OverheadMiB: 512, ComfortPercent: 85,
		BandwidthPerCPUGBs: 4, BandwidthMaxGBs: 25}
}

func TestResolveCPULimits(t *testing.T) {
	tests := []struct {
		name      string
		host      HostFacts
		cpu, mem  int
		ctx       int
		want      CPULimits
		wantError string
	}{
		{"defaults on a 4 cpu 8 GiB node", HostFacts{CPUs: 4, MemTotalBytes: 8 * gib}, 0, 0, 0, CPULimits{CPUMilli: 2000, MemoryMiB: 2048, Context: 4096}, ""},
		{"defaults on a 16 cpu node cap at 2 cpus", HostFacts{CPUs: 16, MemTotalBytes: 32 * gib}, 0, 0, 0, CPULimits{CPUMilli: 2000, MemoryMiB: 8192, Context: 4096}, ""},
		{"defaults on a 1 cpu small node use half a cpu", HostFacts{CPUs: 1, MemTotalBytes: 4 * gib}, 0, 0, 0, CPULimits{CPUMilli: 500, MemoryMiB: 1024, Context: 4096}, ""},
		{"explicit values win", HostFacts{CPUs: 8, MemTotalBytes: 16 * gib}, 3000, 6000, 8192, CPULimits{CPUMilli: 3000, MemoryMiB: 6000, Context: 8192}, ""},
		{"cpu above the node share", HostFacts{CPUs: 4, MemTotalBytes: 16 * gib}, 3500, 0, 0, CPULimits{}, "CPUs is above"},
		{"memory above the node share", HostFacts{CPUs: 4, MemTotalBytes: 8 * gib}, 0, 6000, 0, CPULimits{}, "is above the 60%"},
		{"cap leaves less than the floor", HostFacts{CPUs: 4, MemTotalBytes: 2 * gib}, 0, 1200, 0, CPULimits{}, "floor"},
		{"unknown node needs explicit limits", HostFacts{}, 0, 0, 0, CPULimits{}, "has not reported"},
		{"unknown node with explicit limits passes", HostFacts{}, 2000, 3000, 0, CPULimits{CPUMilli: 2000, MemoryMiB: 3000, Context: 4096}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testPolicy().ResolveCPULimits(tt.host, tt.cpu, tt.mem, tt.ctx)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want invalid containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestCPUCommand(t *testing.T) {
	spec := Spec{Name: "m", Engine: EngineLlamaCpp, ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", Compute: ComputeCPU}
	has := func(cmd []string, flag, val string) bool {
		i := slices.Index(cmd, flag)
		return i >= 0 && (val == "" || (i+1 < len(cmd) && cmd[i+1] == val))
	}
	tests := []struct {
		name      string
		lim       CPULimits
		pol       func(*CPUPolicy)
		file      string
		weights   int64
		embedding bool
		check     func(t *testing.T, cmd []string)
	}{
		{"local file two cpus", CPULimits{CPUMilli: 2000, MemoryMiB: 4096, Context: 4096}, nil, "/models/x.gguf", 2 * gib, false, func(t *testing.T, c []string) {
			if !has(c, "-m", "/models/x.gguf") || has(c, "-hf", "") || !has(c, "-t", "2") || !has(c, "-np", "1") || !has(c, "-c", "4096") {
				t.Errorf("cmd = %v", c)
			}
			if has(c, "-ngl", "") || has(c, "--no-mmap", "") {
				t.Errorf("unexpected -ngl or --no-mmap: %v", c)
			}
			if !has(c, "-cram", "204") {
				t.Errorf("-cram want 204 (5%% of 4096), cmd = %v", c)
			}
		}},
		{"fractional cpu rounds down to one thread", CPULimits{CPUMilli: 1500, MemoryMiB: 4096, Context: 2048}, nil, "/m/x.gguf", 0, false, func(t *testing.T, c []string) {
			if !has(c, "-t", "1") || !has(c, "-tb", "1") {
				t.Errorf("cmd = %v", c)
			}
		}},
		{"sub one cpu still uses one thread", CPULimits{CPUMilli: 500, MemoryMiB: 4096, Context: 2048}, nil, "/m/x.gguf", 0, false, func(t *testing.T, c []string) {
			if !has(c, "-t", "1") {
				t.Errorf("cmd = %v", c)
			}
		}},
		{"cache is bounded on a big node", CPULimits{CPUMilli: 4000, MemoryMiB: 32768, Context: 4096}, nil, "/m/x.gguf", 0, false, func(t *testing.T, c []string) {
			if !has(c, "-cram", "512") {
				t.Errorf("-cram must stay bounded, cmd = %v", c)
			}
		}},
		{"cache has a floor on a tiny cap", CPULimits{CPUMilli: 1000, MemoryMiB: 512, Context: 1024}, nil, "/m/x.gguf", 0, false, func(t *testing.T, c []string) {
			if !has(c, "-cram", "64") {
				t.Errorf("cmd = %v", c)
			}
		}},
		{"tight cap switches to no-mmap", CPULimits{CPUMilli: 2000, MemoryMiB: 2560, Context: 2048}, nil, "/m/x.gguf", 2 * gib, false, func(t *testing.T, c []string) {
			if !has(c, "--no-mmap", "") {
				t.Errorf("cmd = %v", c)
			}
		}},
		{"mmap forced on", CPULimits{CPUMilli: 2000, MemoryMiB: 2560, Context: 2048}, func(p *CPUPolicy) { p.MmapMode = "on" }, "/m/x.gguf", 2 * gib, false, func(t *testing.T, c []string) {
			if has(c, "--no-mmap", "") {
				t.Errorf("cmd = %v", c)
			}
		}},
		{"mmap forced off", CPULimits{CPUMilli: 2000, MemoryMiB: 16000, Context: 2048}, func(p *CPUPolicy) { p.MmapMode = "off" }, "/m/x.gguf", gib, false, func(t *testing.T, c []string) {
			if !has(c, "--no-mmap", "") {
				t.Errorf("cmd = %v", c)
			}
		}},
		{"remote node pulls with -hf", CPULimits{CPUMilli: 2000, MemoryMiB: 4096, Context: 4096}, nil, "", 0, false, func(t *testing.T, c []string) {
			if !has(c, "-hf", spec.ModelRef) || has(c, "-m", "") {
				t.Errorf("cmd = %v", c)
			}
		}},
		{"embedding model", CPULimits{CPUMilli: 1000, MemoryMiB: 1024, Context: 2048}, nil, "/m/x.gguf", 0, true, func(t *testing.T, c []string) {
			if !has(c, "--embedding", "") {
				t.Errorf("cmd = %v", c)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testPolicy()
			if tt.pol != nil {
				tt.pol(&p)
			}
			tt.check(t, CPUCommand(spec, tt.lim, p, tt.file, tt.weights, tt.embedding))
		})
	}
}

func TestCPUPolicyFromEnv(t *testing.T) {
	t.Setenv("APP_MODEL_CPU_SHARES", "64")
	t.Setenv("APP_MODEL_CPU_DEFAULT_CPUS", "1.5")
	t.Setenv("APP_MODEL_CPU_FREE_RAM_FLOOR_MIB", "2048")
	p := LoadCPUPolicy()
	if p.CPUShares != 64 || p.DefaultCPUs != 1.5 || p.FreeRAMFloorMiB != 2048 || p.OomScoreAdj != 800 {
		t.Fatalf("policy = %+v", p)
	}
	l, err := p.ResolveCPULimits(HostFacts{CPUs: 8, MemTotalBytes: 16 * gib}, 0, 0, 0)
	if err != nil || l.CPUMilli != 1500 {
		t.Fatalf("limits = %+v, %v; want 1500 milli from the env default", l, err)
	}
}

func TestCPUBounds(t *testing.T) {
	p := testPolicy()
	b := p.Bounds(HostFacts{CPUs: 8, MemTotalBytes: 16 * gib})
	if b.DefaultCPUMilli != 2000 || b.DefaultMemoryMiB != 4096 || b.MaxCPUMilli != 6000 || b.MaxMemoryMiB != 9830 {
		t.Fatalf("bounds = %+v", b)
	}
	small := p.Bounds(HostFacts{CPUs: 2, MemTotalBytes: 2 * gib})
	if small.MaxMemoryMiB != 1024 {
		t.Errorf("the free RAM floor must cap the ceiling, got %+v", small)
	}
	if unk := p.Bounds(HostFacts{}); unk.MaxCPUMilli != 0 || unk.FreeRAMFloorMiB != 1024 {
		t.Errorf("unknown host = %+v", unk)
	}
}
