package runtimesettings

import "testing"

func TestParseSizePresets(t *testing.T) {
	got, err := ParseSizePresets(DefaultSizePresets)
	if err != nil || len(got) != 4 || got[0].MemoryBytes != 512<<20 || got[3].CPU != 4 {
		t.Fatalf("default table: %+v, %v", got, err)
	}
	for _, bad := range []string{"", "small", "small=0:512Mi", "Small=1:1Gi", "a=1:512", "a=1:1Gi,a=2:2Gi"} {
		if _, err := ParseSizePresets(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
