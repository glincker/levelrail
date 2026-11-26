package telemetry

import (
	"os"
	"regexp"
	"testing"
)

// A db_* name used in the source but missing from the registry would silently roll up as a gauge.
func TestDBMetricsRegistered(t *testing.T) {
	src, err := os.ReadFile("metrickind.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`"(db_[a-z_.]+)"`).FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if name == MetricDBSizePrefix {
			continue
		}
		if _, ok := dbMetricKinds[name]; !ok {
			t.Errorf("db metric %q is not registered in dbMetricKinds", name)
		}
	}
	for name, kind := range dbMetricKinds {
		if KindOf(name) != kind {
			t.Errorf("KindOf(%q) = %v, want %v", name, KindOf(name), kind)
		}
	}
	if KindOf(MetricDBSizePrefix+"app") != KindGauge || !IsDBMetric(MetricDBSizePrefix+"app") {
		t.Error("size series must be a registered gauge")
	}
	if IsDBMetric("db_unknown") {
		t.Error("unregistered db_ name accepted")
	}
}

func TestKindOfUnknownDBMetricPanicsInTests(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("unregistered db_ metric must fail in tests")
		}
	}()
	KindOf("db_not_registered")
}
