package resourceadvisor

import "testing"

func TestNodePriceSavings(t *testing.T) {
	const gib = int64(1 << 30)
	mem := map[string]float64{"n1": float64(4 * gib)}
	mk := func() []Finding {
		return []Finding{{ID: "a", Kind: KindMemoryLimitMissing, Resource: app("web"), Impact: Impact{SavingMemoryBytes: gib}}}
	}
	in := Input{Resources: []Resource{{Ref: app("web"), NodeID: "n1"}}}

	fs := mk()
	applyNodePrices(fs, in, mem)
	if fs[0].Impact.MonthlySaving != nil {
		t.Fatal("no price set must leave the saving unknown")
	}
	if got := Summarize(fs, 3).MonthlySavings; got != nil {
		t.Fatalf("summary must stay null without a price, got %v", *got)
	}

	in.NodePrices = map[string]float64{"n1": 20}
	fs = mk()
	applyNodePrices(fs, in, mem)
	if fs[0].Impact.MonthlySaving == nil || *fs[0].Impact.MonthlySaving != 5 {
		t.Fatalf("1 of 4 GiB on a 20 node should save 5, got %v", fs[0].Impact.MonthlySaving)
	}
	if got := Summarize(fs, 3).MonthlySavings; got == nil || *got != 5 {
		t.Fatalf("summary should sum to 5, got %v", got)
	}
}
