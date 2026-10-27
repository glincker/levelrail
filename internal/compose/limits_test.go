package compose

import "testing"

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
