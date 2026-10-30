package costestimate

import "testing"

func TestSavingFromRightsizing(t *testing.T) {
	const gib = int64(1) << 30
	table := DefaultRateTable()

	// 4 GiB + 2 cores to 1 GiB + 1 core on the vm rates: (16+8) - (4+4).
	got, ok := Saving(4*gib, 2_000_000_000, gib, 1_000_000_000, table)
	if !ok || got.USD != 16 || got.ProviderKey != ProviderVM {
		t.Fatalf("Saving = %+v ok=%v, want 16 USD on vm", got, ok)
	}
	if _, ok := Saving(gib, 1_000_000_000, 2*gib, 1_000_000_000, table); ok {
		t.Error("a larger suggestion must not report a saving")
	}
	if _, ok := Saving(0, 0, gib, 1_000_000_000, table); ok {
		t.Error("no declared size must report no saving")
	}
}
