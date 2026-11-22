package imagelife

import "testing"

func TestEstimateReclaimWidensWhenRemovedImagesShare(t *testing.T) {
	one := EstimateReclaim([]Entry{{SizeBytes: 100, SharedBytes: 40}, {SizeBytes: 50}})
	if !one.Exact || one.MinBytes != 110 || one.MaxBytes != 110 {
		t.Fatalf("one sharer = %+v", one)
	}
	two := EstimateReclaim([]Entry{{SizeBytes: 100, SharedBytes: 40}, {SizeBytes: 80, SharedBytes: 40}})
	if two.Exact || two.MinBytes != 100 || two.MaxBytes != 180 {
		t.Fatalf("two sharers = %+v", two)
	}
}
