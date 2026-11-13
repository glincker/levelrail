package cloudiac

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func fullSpec() OCISpec {
	return OCISpec{
		Name: "lr-test", Region: "us-phoenix-1", CompartmentOCID: "ocid1.compartment.oc1..example", Namespace: "ns",
		Networks:  []NetworkSpec{{Name: "lr-net", CIDR: "10.30.0.0/16", IngressPorts: []int{443, 80}}},
		Buckets:   []BucketSpec{{Name: "lr-data"}, {Name: "lr-scratch", Unprotected: true}},
		Instances: []InstanceSpec{{Name: "worker-1", Network: "lr-net", Shape: "VM.Standard.A1.Flex", OCPUs: 1, MemoryGB: 6, AvailabilityDomain: "AD-1", ImageOCID: "ocid1.image.oc1..example"}},
	}
}

func TestRenderOCIGolden(t *testing.T) {
	cases := map[string]OCISpec{
		"network_only": {Name: "net", Region: "us-phoenix-1", CompartmentOCID: "c", Networks: []NetworkSpec{{Name: "n1", CIDR: "10.0.0.0/24", IngressPorts: []int{22}}}},
		"bucket_only":  {Name: "bkt", Region: "us-phoenix-1", CompartmentOCID: "c", Namespace: "ns", Buckets: []BucketSpec{{Name: "data"}}},
		"full":         fullSpec(),
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := RenderOCI(spec)
			if err != nil {
				t.Fatal(err)
			}
			got := m.Files[MainFile]
			path := filepath.Join("testdata", "golden", name+".tf.json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path) //nolint:gosec // fixed golden path
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("golden mismatch for %s, rerun with -update", name)
			}
		})
	}
}

func TestRenderOCIDeterministicNoCredentials(t *testing.T) {
	a, _ := RenderOCI(fullSpec())
	for i := 0; i < 5; i++ {
		b, _ := RenderOCI(fullSpec())
		if !bytes.Equal(a.Files[MainFile], b.Files[MainFile]) {
			t.Fatal("output not deterministic")
		}
	}
	for _, banned := range []string{"private_key", "fingerprint", "tenancy_ocid", "user_ocid", "password", "token"} {
		if strings.Contains(string(a.Files[MainFile]), banned) {
			t.Fatalf("rendered config mentions %q", banned)
		}
	}
	if a.Provider != "oracle" || a.Variables["region"] != "us-phoenix-1" || len(a.Files) != 1 {
		t.Fatalf("unexpected module: %+v", a)
	}
}

func TestRenderOCIInvalid(t *testing.T) {
	mut := func(f func(*OCISpec)) OCISpec { s := fullSpec(); f(&s); return s }
	cases := map[string]OCISpec{
		"bad module name":  mut(func(s *OCISpec) { s.Name = "Bad Name" }),
		"no region":        mut(func(s *OCISpec) { s.Region = "" }),
		"bad cidr":         mut(func(s *OCISpec) { s.Networks[0].CIDR = "nope" }),
		"bad port":         mut(func(s *OCISpec) { s.Networks[0].IngressPorts = []int{70000} }),
		"hcl injection":    mut(func(s *OCISpec) { s.Buckets[0].Name = `x" { }` }),
		"unknown network":  mut(func(s *OCISpec) { s.Instances[0].Network = "ghost" }),
		"duplicate bucket": mut(func(s *OCISpec) { s.Buckets[1].Name = s.Buckets[0].Name }),
		"no namespace":     mut(func(s *OCISpec) { s.Namespace = "" }),
		"no image":         mut(func(s *OCISpec) { s.Instances[0].ImageOCID = "" }),
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := RenderOCI(spec)
			var ce *Error
			if !errors.As(err, &ce) || ce.Kind != ErrInvalidSpec {
				t.Fatalf("want invalid_spec, got %v", err)
			}
		})
	}
}
