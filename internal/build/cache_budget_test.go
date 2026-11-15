package build

import (
	"context"
	"testing"

	bkclient "github.com/moby/buildkit/client"

	"github.com/GLINCKER/levelrail/internal/build/buildkitd"
)

type budgetSpy struct{ info bkclient.PruneInfo }

func (b *budgetSpy) Prune(_ context.Context, ch chan bkclient.UsageInfo, opts ...bkclient.PruneOption) error {
	for _, o := range opts {
		o.SetPruneOption(&b.info)
	}
	ch <- bkclient.UsageInfo{Size: 300}
	return nil
}

func TestPruneToBudgetKeepsReservedAndMaxWithNoMinFree(t *testing.T) {
	spy := &budgetSpy{}
	res, err := pruneToBudget(context.Background(), spy, buildkitd.Budget{ReservedBytes: 2 << 30, MaxUsedBytes: 10 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if !spy.info.All || spy.info.ReservedSpace != 2<<30 || spy.info.MaxUsedSpace != 10<<30 || spy.info.MinFreeSpace != 0 {
		t.Fatalf("prune info = %+v", spy.info)
	}
	if res.FreedBytes != 300 || res.Removed != 1 {
		t.Fatalf("res = %+v", res)
	}
	if IsDesktopEngine("Ubuntu 24.04 LTS") || !IsDesktopEngine("Docker Desktop") {
		t.Fatal("desktop detection")
	}
	if b := buildkitd.BudgetFor(0, 0, 0); b.MaxUsedBytes != 5120<<20 {
		t.Fatalf("unknown disk budget = %d", b.MaxUsedBytes)
	}
}
