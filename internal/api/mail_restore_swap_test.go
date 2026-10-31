package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mailbackup/moxrun"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeHolder struct {
	hold string
	log  *[]string
}

func (f *fakeHolder) UpdateServiceMaintenanceHold(_ context.Context, _ string, hold string) error {
	f.hold = hold
	*f.log = append(*f.log, "hold="+hold)
	return nil
}

// swapRuntime models the mail container, the helper, and a reconciler pass
// that starts any stopped container unless the app is held.
type swapRuntime struct {
	docker.Runtime
	holder          *fakeHolder
	running         bool
	log             []string
	startMidSwap    bool
	failRollback    bool
	state           string
	reconcileStarts int
}

func (f *swapRuntime) pass() {
	if f.holder.hold == "" && !f.running {
		f.running = true
		f.reconcileStarts++
	}
}

func (f *swapRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	if name == moxrun.HelperName("mail") {
		return nil, nil
	}
	return &docker.ContainerState{ID: "mail-1", Name: name, Running: f.running}, nil
}

func (f *swapRuntime) Stop(context.Context, string, time.Duration) error {
	f.running = false
	f.log = append(f.log, "stop")
	f.pass()
	return nil
}

func (f *swapRuntime) Start(_ context.Context, id string) error {
	if id == "mail-1" {
		f.running = true
		f.log = append(f.log, "start")
	}
	return nil
}

func (f *swapRuntime) Create(context.Context, docker.ContainerSpec) (string, error) {
	return "helper-1", nil
}

func (f *swapRuntime) Remove(context.Context, string, bool) error { return nil }

func (f *swapRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	mode := cmd[4] + " " + cmd[5]
	f.log = append(f.log, mode)
	switch cmd[4] {
	case "swap":
		f.pass()
		if f.startMidSwap {
			f.running = true
		}
	case "rollback":
		if f.failRollback {
			return nil, errors.New("mv failed")
		}
	case "state":
		return io.NopCloser(strings.NewReader(f.state + "\n")), nil
	}
	return io.NopCloser(strings.NewReader("ok\n")), nil
}

func newTestSwap(f *swapRuntime) *mailSwap {
	svc := store.DesiredService{Name: "mail", Image: "mox:v1"}
	vols := moxrun.Volumes{Data: "mail_data", Config: "mail_config"}
	return &mailSwap{
		rt: f, holder: f.holder, vols: vols, roots: rootsFor(vols, true), step: func(string) {},
		t:      mailBackupTarget{svc: svc, container: docker.ContainerState{ID: "mail-1", Name: "mail-1"}, rt: f},
		logger: slog.New(slog.DiscardHandler), ready: func(context.Context) error { return nil },
	}
}

func newSwapRuntime() *swapRuntime {
	f := &swapRuntime{running: true}
	f.holder = &fakeHolder{log: &f.log}
	return f
}

func TestMailSwapHoldKeepsReconcilerFromStartingMail(t *testing.T) {
	f := newSwapRuntime()
	rb, err := newTestSwap(f).run(context.Background(), "20261030T120000Z")
	if err != nil || rb != "" {
		t.Fatalf("run = %q, %v", rb, err)
	}
	if f.reconcileStarts != 0 {
		t.Fatalf("reconciler started mail %d times during the swap", f.reconcileStarts)
	}
	got := strings.Join(f.log, ",")
	want := "hold=mail-restore:20261030T120000Z,stop,swap /mox/data,swap /mox/config,start,commit /mox/data,commit /mox/config,hold="
	if got != want {
		t.Fatalf("order\n got %s\nwant %s", got, want)
	}
}

func TestMailSwapStartedMidSwapRollsBack(t *testing.T) {
	f := newSwapRuntime()
	f.startMidSwap = true
	rb, err := newTestSwap(f).run(context.Background(), "20261030T120000Z")
	if err == nil || rb != mailRollbackDone {
		t.Fatalf("run = %q, %v; want a rolled back failure", rb, err)
	}
	if !strings.Contains(strings.Join(f.log, ","), "rollback /mox/config,rollback /mox/data") || f.holder.hold != "" {
		t.Fatalf("want both roots rolled back in reverse and the hold released: %v hold=%q", f.log, f.holder.hold)
	}
}

func TestMailSwapFailedRollbackKeepsHold(t *testing.T) {
	f := newSwapRuntime()
	f.startMidSwap, f.failRollback = true, true
	rb, err := newTestSwap(f).run(context.Background(), "20261030T120000Z")
	if err == nil || rb != mailRollbackFailed || f.holder.hold == "" {
		t.Fatalf("run = %q, %v hold=%q; want rollback_failed and the hold kept", rb, err, f.holder.hold)
	}
}

func TestMailSwapRecover(t *testing.T) {
	f := newSwapRuntime()
	f.running, f.state, f.holder.hold = false, "20261030T120000Z pre in", "mail-restore:20261030T120000Z"
	rb, err := newTestSwap(f).recover(context.Background())
	if rb != mailRollbackDone || err == nil || f.holder.hold != "" || !f.running {
		t.Fatalf("open journal: %q %v hold=%q running=%v", rb, err, f.holder.hold, f.running)
	}
	f = newSwapRuntime()
	f.running, f.state, f.holder.hold = false, "none", "mail-restore:20261030T120000Z"
	rb, err = newTestSwap(f).recover(context.Background())
	if rb != "" || err != nil || f.holder.hold != "" || !f.running || strings.Contains(strings.Join(f.log, ","), "rollback") {
		t.Fatalf("closed data journal must complete, not roll back: %q %v %v", rb, err, f.log)
	}
}
