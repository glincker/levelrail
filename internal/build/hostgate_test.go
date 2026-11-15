package build

import (
	"context"
	"testing"
)

type fakeGate struct{ waited int }

func (f *fakeGate) Admit(_ context.Context, wait func(string)) error {
	f.waited++
	wait("waiting: host under memory pressure")
	return nil
}
func (f *fakeGate) BuildCap(int) int { return 1 }

func TestAdmitLocalGatesOnlyLocalBuilds(t *testing.T) {
	g := &fakeGate{}
	SetHostGate(g)
	t.Cleanup(func() { SetHostGate(nil) })
	var logs []string
	n, err := admitLocal(context.Background(), "", 4, func(e ProgressEvent) { logs = append(logs, e.Log) })
	if err != nil || n != 1 || len(logs) != 1 {
		t.Fatalf("local build: n=%d err=%v logs=%v", n, err, logs)
	}
	n, err = admitLocal(context.Background(), "node-b", 4, nil)
	if err != nil || n != 4 || g.waited != 1 {
		t.Fatalf("remote build must not be gated: n=%d err=%v waited=%d", n, err, g.waited)
	}
}

func TestDerivedMaxConcurrent(t *testing.T) {
	for _, c := range []struct{ cpu, mem, want int }{{2, 8192, 1}, {1, 1024, 1}, {16, 65536, 8}, {16, 8192, 2}} {
		if got := DerivedMaxConcurrent(c.cpu, c.mem); got != c.want {
			t.Errorf("cpus=%d mem=%d got %d want %d", c.cpu, c.mem, got, c.want)
		}
	}
}

func TestAdmitImagePullGate(t *testing.T) {
	SetHostGate(nil)
	if err := AdmitImagePull(context.Background(), nil); err != nil {
		t.Fatalf("no gate must admit: %v", err)
	}
	g := &fakeGate{}
	SetHostGate(g)
	t.Cleanup(func() { SetHostGate(nil) })
	var msgs []string
	if err := AdmitImagePull(context.Background(), func(m string) { msgs = append(msgs, m) }); err != nil || len(msgs) != 1 {
		t.Fatalf("gated pull: err=%v msgs=%v", err, msgs)
	}
}
