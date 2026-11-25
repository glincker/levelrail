package api

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

type fakeLiveRestore struct {
	fakeVersionPinger
	on, swarm bool
	err       error
}

func (f fakeLiveRestore) LiveRestorePosture(context.Context) (bool, bool, error) {
	return f.on, f.swarm, f.err
}

func TestDoctorCheckLiveRestore(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("check is Linux only")
	}
	run := func(f fakeLiveRestore) doctorCheckResource {
		return (&Router{dockerPinger: f}).doctorCheckLiveRestore(context.Background())
	}
	off := run(fakeLiveRestore{})
	if off.Status != doctorStatusWarn || !strings.Contains(off.Fix, "systemctl reload docker") || !strings.Contains(off.Fix, "swarm") {
		t.Errorf("off: %v %q", off.Status, off.Fix)
	}
	if got := run(fakeLiveRestore{on: true}); got.Status != doctorStatusOK {
		t.Errorf("on: %v", got.Status)
	}
	if got := run(fakeLiveRestore{swarm: true}); got.Status != doctorStatusUnknown {
		t.Errorf("swarm: %v", got.Status)
	}
	if got := run(fakeLiveRestore{err: context.Canceled}); got.Status != doctorStatusUnknown {
		t.Errorf("err: %v", got.Status)
	}
	if got := (&Router{}).doctorCheckLiveRestore(context.Background()); got.Status != doctorStatusUnknown {
		t.Errorf("no client: %v", got.Status)
	}
}
