package exposure

import (
	"context"
	"errors"
	"testing"
	"time"
)

// manualTimers collects scheduled callbacks so a test fires them by hand.
type manualTimers struct {
	now     time.Time
	fns     []func()
	stopped int
}

func (t *manualTimers) after(_ time.Duration, f func()) func() bool {
	t.fns = append(t.fns, f)
	return func() bool { t.stopped++; return true }
}

func trialManager(f *fakeChain) (*Manager, *manualTimers) {
	m := newTestManager(f)
	tm := &manualTimers{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	m.SetClock(func() time.Time { return tm.now }, tm.after)
	return m, tm
}

func TestTrialRevertsWhenUnconfirmed(t *testing.T) {
	f := &fakeChain{rules: []string{"-j RETURN"}}
	m, tm := trialManager(f)
	var reverted []Change
	m.OnRevert(func(c Change, err error) {
		if err != nil {
			t.Errorf("revert error: %v", err)
		}
		reverted = append(reverted, c)
	})
	ch, err := m.ApplyWithRevert(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.2"}}, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if want := tm.now.Add(time.Minute); !ch.ExpiresAt.Equal(want) {
		t.Fatalf("expires = %v, want %v", ch.ExpiresAt, want)
	}
	if len(f.rules) != 3 || len(m.Pending()) != 1 {
		t.Fatalf("rules = %v pending = %v", f.rules, m.Pending())
	}
	tm.fns[0]()
	if len(f.rules) != 1 || f.rules[0] != "-j RETURN" {
		t.Fatalf("rules after revert = %v", f.rules)
	}
	if len(reverted) != 1 || reverted[0].Port != 8108 || len(m.Pending()) != 0 {
		t.Fatalf("revert callback = %v pending = %v", reverted, m.Pending())
	}
}

func TestTrialRestoresPreviousRestriction(t *testing.T) {
	f := &fakeChain{rules: []string{"-j RETURN"}}
	m, tm := trialManager(f)
	prev := Restriction{Port: 8108, Allow: []string{"10.0.0.2"}}
	if _, err := m.Apply(context.Background(), prev); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyWithRevert(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.9"}}, &prev, time.Minute); err != nil {
		t.Fatal(err)
	}
	tm.fns[0]()
	chain := m.ReadChain(context.Background())
	rules := m.tagged(chain, m.tag("tcp", 8108))
	if len(rules) != 2 || rules[0].Source != "10.0.0.2/32" {
		t.Fatalf("previous not restored: %v", f.rules)
	}
}

func TestTrialConfirmKeepsRulesAndStopsTimer(t *testing.T) {
	f := &fakeChain{}
	m, tm := trialManager(f)
	if _, err := m.ApplyWithRevert(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.2"}}, nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	ch, ok := m.Confirm("tcp", 8108)
	if !ok || ch.Port != 8108 || tm.stopped != 1 {
		t.Fatalf("confirm = %v %v stopped=%d", ch, ok, tm.stopped)
	}
	tm.fns[0]() // a late timer must find nothing to revert
	if len(f.rules) != 2 {
		t.Fatalf("confirmed rules were reverted: %v", f.rules)
	}
	if _, ok := m.Confirm("tcp", 8108); ok {
		t.Fatal("second confirm should find nothing")
	}
}

func TestSyncLeavesPendingAndRestartDropsIt(t *testing.T) {
	f := &fakeChain{}
	m, _ := trialManager(f)
	want := Restriction{Port: 8108, Allow: []string{"10.0.0.2"}}
	if _, err := m.ApplyWithRevert(context.Background(), want, nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	// The reconciler runs during the window with nothing stored: the trial must survive.
	if res := m.Sync(context.Background(), nil); res.Removed != 0 || len(f.rules) != 2 {
		t.Fatalf("sync touched a pending rule: %+v rules=%v", res, f.rules)
	}
	// A restarted control plane has no pending state, so the same pass rolls it back.
	restarted := newTestManager(f)
	if res := restarted.Sync(context.Background(), nil); res.Removed != 1 || len(f.rules) != 0 {
		t.Fatalf("restart did not revert: %+v rules=%v", res, f.rules)
	}
}

func TestTrialRefusesLockoutAndBadWindow(t *testing.T) {
	m, _ := trialManager(&fakeChain{})
	if _, err := m.ApplyWithRevert(context.Background(), Restriction{Port: 22, Allow: []string{"10.0.0.2"}}, nil, time.Minute); !errors.Is(err, ErrLockout) {
		t.Fatalf("err = %v, want ErrLockout", err)
	}
	if _, err := m.ApplyWithRevert(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.2"}}, nil, 0); err == nil {
		t.Fatal("zero window accepted")
	}
}

func TestGuardDetectedSSHPortAndCaller(t *testing.T) {
	m := newTestManager(&fakeChain{})
	m.SetSSHProbe(func(context.Context) []int { return []int{2222} })
	r := Restriction{Port: 2222, Protocol: "tcp", Allow: []string{"10.0.0.0/8"}}
	if got := m.Guard(context.Background(), r, "10.1.1.1"); len(got) != 1 {
		t.Fatalf("sshd port reasons = %v", got)
	}
	r = Restriction{Port: 8108, Protocol: "tcp", Allow: []string{"10.0.0.0/8"}}
	if got := m.Guard(context.Background(), r, "10.1.1.1"); len(got) != 0 {
		t.Fatalf("covered caller refused: %v", got)
	}
	if got := m.Guard(context.Background(), r, "203.0.113.5"); len(got) != 1 {
		t.Fatalf("uncovered caller reasons = %v", got)
	}
	for _, caller := range []string{"127.0.0.1", "", "not-an-ip", "2001:db8::1"} {
		if got := m.Guard(context.Background(), r, caller); len(got) != 0 {
			t.Errorf("caller %q refused: %v", caller, got)
		}
	}
}

func TestParseSSHPortSources(t *testing.T) {
	if got := parseSSHDConfig("port 22\nport 2222\nlistenaddress 0.0.0.0\n"); len(got) != 2 || got[1] != 2222 {
		t.Fatalf("sshd -T ports = %v", got)
	}
	ss := `LISTEN 0 128 0.0.0.0:2200 0.0.0.0:* users:(("sshd",pid=1,fd=3))
LISTEN 0 128 [::]:2200 [::]:* users:(("sshd",pid=1,fd=4))
LISTEN 0 128 0.0.0.0:5432 0.0.0.0:* users:(("postgres",pid=2,fd=5))`
	if got := parseSSListeners(ss); len(got) != 2 || got[0] != 2200 {
		t.Fatalf("ss ports = %v", got)
	}
}
