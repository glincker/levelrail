package compose

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToDesiredServices_AppliesResourceLimits(t *testing.T) {
	f, err := Parse([]byte(`services:
  a:
    image: x:1
    mem_limit: 512m
    cpus: 1.5
  b:
    image: x:1
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 1G
`))
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int64{"app-a": {512 << 20, 1_500_000_000}, "app-b": {1 << 30, 500_000_000}}
	for _, svc := range out {
		w := want[svc.Name]
		if svc.Resources == nil || svc.Resources.MemoryBytes != w[0] || svc.Resources.NanoCPUs != w[1] {
			t.Errorf("%s resources = %+v, want mem %d nano %d", svc.Name, svc.Resources, w[0], w[1])
		}
	}
	if len(f.Unsupported) != 0 {
		t.Errorf("limits must not be reported as ignored: %v", f.Unsupported)
	}
}

func TestToDesiredServices_RejectsBadMemoryLimit(t *testing.T) {
	f, err := Parse([]byte("services:\n  a:\n    image: x:1\n    mem_limit: lots\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ToDesiredServices("app", f); err == nil {
		t.Fatal("want an error for an unparseable memory limit")
	}
}

func TestLimitsForService_ValidAndInvalid(t *testing.T) {
	t.Setenv("APP_COMPOSE_MAX_MEMORY_BYTES", "")
	t.Setenv("APP_COMPOSE_MAX_CPUS", "")
	ok := []struct {
		mem, cpu string
		wantMem  int64
		wantNano int64
	}{
		{"512m", "", 512 << 20, 0},
		{"1g", "", 1 << 30, 0},
		{"1.5g", "", 3 << 29, 0},
		{"100m", "", 100 << 20, 0},
		{"", "0.5", 0, 500_000_000},
		{"", "2", 0, 2_000_000_000},
	}
	for _, c := range ok {
		mem, nano, err := limitsForService(Service{MemLimit: yamlScalarString(c.mem), CPUs: yamlScalarString(c.cpu)})
		if err != nil || mem != c.wantMem || nano != c.wantNano {
			t.Errorf("%q/%q = %d,%d,%v want %d,%d", c.mem, c.cpu, mem, nano, err, c.wantMem, c.wantNano)
		}
	}
	for _, mem := range []string{"nan", "inf", "-inf", "1e30", "9999999999g", "0.0000001m", "0", "-5m", "0.4b"} {
		if _, _, err := limitsForService(Service{MemLimit: yamlScalarString(mem)}); err == nil {
			t.Errorf("mem_limit %q must be rejected", mem)
		} else if !strings.Contains(err.Error(), "mem_limit") {
			t.Errorf("error for %q must name the key: %v", mem, err)
		}
	}
	for _, cpu := range []string{"nan", "NaN", "inf", "+Inf", "1e12", "1e-10", "0", "-1", "1e30"} {
		if _, _, err := limitsForService(Service{CPUs: yamlScalarString(cpu)}); err == nil {
			t.Errorf("cpus %q must be rejected", cpu)
		} else if !strings.Contains(err.Error(), "cpus") {
			t.Errorf("error for %q must name the key: %v", cpu, err)
		}
	}
}

func TestLimitsForService_EnvCap(t *testing.T) {
	t.Setenv("APP_COMPOSE_MAX_MEMORY_BYTES", "1073741824")
	t.Setenv("APP_COMPOSE_MAX_CPUS", "4")
	if _, _, err := limitsForService(Service{MemLimit: "2g"}); err == nil {
		t.Error("mem above env cap must be rejected")
	}
	if _, _, err := limitsForService(Service{CPUs: "8"}); err == nil {
		t.Error("cpus above env cap must be rejected")
	}
	if _, _, err := limitsForService(Service{MemLimit: "1g", CPUs: "4"}); err != nil {
		t.Errorf("values at the cap are valid: %v", err)
	}
}

func TestLimitsForService_ShorthandWinsOverDeploy(t *testing.T) {
	t.Setenv("APP_COMPOSE_MAX_MEMORY_BYTES", "")
	t.Setenv("APP_COMPOSE_MAX_CPUS", "")
	svc := Service{MemLimit: "256m", CPUs: "1", Deploy: &Deploy{}}
	svc.Deploy.Resources.Limits = deployLimits{Memory: "1g", CPUs: "2"}
	mem, nano, err := limitsForService(svc)
	if err != nil || mem != 256<<20 || nano != 1_000_000_000 {
		t.Errorf("got %d,%d,%v", mem, nano, err)
	}
}

func TestToDesiredServices_ShmSize(t *testing.T) {
	t.Setenv("APP_COMPOSE_MAX_SHM_BYTES", "")
	parse := func(v string) (*store.DesiredService, error) {
		f, err := Parse([]byte("services:\n  a:\n    image: x:1\n    shm_size: " + v + "\n"))
		if err != nil {
			return nil, err
		}
		if len(f.Unsupported) != 0 {
			t.Errorf("shm_size must not be reported as ignored: %v", f.Unsupported)
		}
		out, _, err := ToDesiredServices("app", f)
		if err != nil {
			return nil, err
		}
		return &out[0], nil
	}
	for raw, want := range map[string]int64{"1gb": 1 << 30, "256m": 256 << 20, "\"2g\"": 2 << 30, "67108864": 64 << 20} {
		svc, err := parse(raw)
		if err != nil || svc.Resources == nil || svc.Resources.ShmSizeBytes != want {
			t.Errorf("shm_size %s = %+v err %v, want %d", raw, svc, err, want)
		}
	}
	for _, bad := range []string{"lots", "0", "-1g", "NaN", "Inf", "0.1b", "99999999999g"} {
		if _, err := parse(bad); err == nil || !strings.Contains(err.Error(), "shm_size") {
			t.Errorf("shm_size %s: err = %v, want a shm_size error", bad, err)
		}
	}
	t.Setenv("APP_COMPOSE_MAX_SHM_BYTES", "536870912")
	if _, err := parse("1g"); err == nil || !strings.Contains(err.Error(), "APP_COMPOSE_MAX_SHM_BYTES") {
		t.Errorf("1g over a 512 MiB cap: err = %v, want the env cap named", err)
	}
	if svc, err := parse("512m"); err != nil || svc.Resources.ShmSizeBytes != 512<<20 {
		t.Errorf("512m at the cap: %+v %v", svc, err)
	}
}

func TestShmSizeDefaultCap(t *testing.T) {
	t.Setenv("APP_COMPOSE_MAX_SHM_BYTES", "")
	parse := func(v string) error {
		f, err := Parse([]byte("services:\n  a:\n    image: x:1\n    shm_size: " + v + "\n"))
		if err != nil {
			return err
		}
		_, _, err = ToDesiredServices("app", f)
		return err
	}
	if err := parse("500g"); err == nil || !strings.Contains(err.Error(), "APP_COMPOSE_MAX_SHM_BYTES") {
		t.Fatalf("500g with no override: err = %v, want the default cap named", err)
	}
	t.Setenv("APP_COMPOSE_MAX_SHM_BYTES", "8589934592")
	if err := parse("8g"); err != nil {
		t.Fatalf("8g under a raised cap: %v", err)
	}
}
