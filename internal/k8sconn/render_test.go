package k8sconn

import (
	"os"
	"strings"
	"testing"
)

func baseSpec() AppSpec {
	uid := int64(10001)
	return AppSpec{
		Name: "whoami", Namespace: "apps", Image: "traefik/whoami:v1.10", Port: 80,
		Replicas: 2, Env: map[string]string{"B": "2", "A": "1"},
		Host: "whoami.example.com", IngressClass: "traefik", RunAsUser: &uid,
		Resources: Resources{CPURequest: "50m", MemoryRequest: "64Mi", CPULimit: "500m", MemoryLimit: "256Mi"},
	}
}

func TestRenderGolden(t *testing.T) {
	noHost := baseSpec()
	noHost.Host, noHost.IngressClass, noHost.RunAsUser, noHost.Env = "", "", nil, nil
	tls := baseSpec()
	tls.TLSIssuer = "letsencrypt"

	cases := map[string]AppSpec{"basic": baseSpec(), "nohost": noHost, "tls": tls}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := Render(spec)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.YAML()
			if err != nil {
				t.Fatal(err)
			}
			path := "testdata/" + name + ".golden.yaml"
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("golden mismatch for %s:\n%s", name, got)
			}
			s := string(got)
			for _, bad := range []string{"hostPath", "hostNetwork: true", "privileged: true"} {
				if strings.Contains(s, bad) {
					t.Fatalf("unsafe field %q rendered", bad)
				}
			}
			if strings.Count(s, LabelManagedBy+": levelrail") < 2 {
				t.Fatal("objects missing managed-by label")
			}
		})
	}
}

func TestRenderValidation(t *testing.T) {
	bad := []func(*AppSpec){
		func(s *AppSpec) { s.Name = "Bad_Name" },
		func(s *AppSpec) { s.Image = " " },
		func(s *AppSpec) { s.Port = 0 },
		func(s *AppSpec) { s.Resources.MemoryLimit = "" },
		func(s *AppSpec) { s.Namespace = "" },
	}
	for i, mut := range bad {
		s := baseSpec()
		mut(&s)
		if _, err := Render(s); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}
