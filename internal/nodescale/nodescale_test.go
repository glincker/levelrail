package nodescale

import (
	"context"
	"testing"
	"time"
)

type fakeSrc struct{ nodes []Node }

func (f fakeSrc) Nodes(context.Context) ([]Node, error) { return f.nodes, nil }

type fakeSettings struct {
	on   bool
	mins int
}

func (f fakeSettings) Enabled(context.Context) bool    { return f.on }
func (f fakeSettings) IdleMinutes(context.Context) int { return f.mins }

func threeNodes() []Node {
	on := map[string]string{LabelAutoscale: LabelAutoscaleOn}
	return []Node{
		{ID: "cp", Name: "cp", Provisioned: true, ControlPlane: true, Labels: on},
		{ID: "empty", Name: "empty", Provisioned: true, Labels: on, MonthlyPrice: 12},
		{ID: "last", Name: "last", Pools: []string{"gpu"}, Provisioned: true, Labels: on},
	}
}

func TestTickEmitsOneWouldDestroy(t *testing.T) {
	var got []Event
	w := New(fakeSrc{threeNodes()}, fakeSettings{true, 60}, time.Minute, func(e Event) { got = append(got, e) })
	t0 := time.Now()
	if ev := w.Tick(context.Background(), t0); len(ev) != 0 {
		t.Fatalf("first pass only starts idle timers, got %v", ev)
	}
	w.Tick(context.Background(), t0.Add(61*time.Minute))
	w.Tick(context.Background(), t0.Add(62*time.Minute))
	if len(got) != 1 || got[0].NodeID != "empty" || got[0].Kind != EventWouldDestroy {
		t.Fatalf("want one would_destroy for empty, got %+v", got)
	}
	if got[0].MonthlyPrice == nil || *got[0].MonthlyPrice != 12 {
		t.Fatalf("price missing: %+v", got[0])
	}
}

func TestOffDoesNothing(t *testing.T) {
	w := New(fakeSrc{threeNodes()}, fakeSettings{false, 1}, time.Minute, nil)
	t0 := time.Now()
	w.Tick(context.Background(), t0)
	if ev := w.Tick(context.Background(), t0.Add(time.Hour)); len(ev) != 0 {
		t.Fatalf("got %v", ev)
	}
}

func TestVolumeHolderAndBusyNodesSkipped(t *testing.T) {
	n := threeNodes()
	n[1].HoldsVolumes = true
	w := New(fakeSrc{n}, fakeSettings{true, 1}, time.Minute, nil)
	t0 := time.Now()
	w.Tick(context.Background(), t0)
	if ev := w.Tick(context.Background(), t0.Add(time.Hour)); len(ev) != 0 {
		t.Fatalf("got %v", ev)
	}
}
