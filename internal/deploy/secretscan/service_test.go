package secretscan

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newSQL(t *testing.T) *SQLStore {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLStore(db)
}

func newService(t *testing.T, scan func(context.Context, string, Limits) (Result, error)) (*Service, *SQLStore) {
	t.Helper()
	st := newSQL(t)
	opts := []Option{}
	if scan != nil {
		opts = append(opts, WithDirScanner(scan))
	}
	return New(st, DefaultLimits(), nil, opts...), st
}

const fp = "0123456789abcdef"

func findingScan() func(context.Context, string, Limits) (Result, error) {
	return func(context.Context, string, Limits) (Result, error) {
		return Result{FilesScanned: 3, Findings: []Finding{{RuleID: "github-token", Source: SourceFile, Path: "a.env", Line: 4, Fingerprint: fp}}}, nil
	}
}

func TestBeforeDeployPolicies(t *testing.T) {
	cases := []struct {
		name        string
		policy      Policy
		scan        func(context.Context, string, Limits) (Result, error)
		wantBlocked bool
		wantAction  string
		wantStored  bool
	}{
		{"off skips", PolicyOff, findingScan(), false, "", false},
		{"warn with finding", PolicyWarn, findingScan(), false, ActionWarn, true},
		{"block with finding", PolicyBlock, findingScan(), true, ActionBlock, true},
		{"block clean", PolicyBlock, func(context.Context, string, Limits) (Result, error) { return Result{FilesScanned: 1}, nil }, false, ActionAllow, true},
		{"block fails closed on scanner error", PolicyBlock, func(context.Context, string, Limits) (Result, error) {
			return Result{}, errors.New("disk exploded")
		}, true, ActionBlock, true},
		{"warn on scanner error warns", PolicyWarn, func(context.Context, string, Limits) (Result, error) {
			return Result{}, errors.New("disk exploded")
		}, false, ActionWarn, true},
		{"block truncated clean warns", PolicyBlock, func(context.Context, string, Limits) (Result, error) {
			return Result{Truncated: true}, nil
		}, false, ActionWarn, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, st := newService(t, c.scan)
			ctx := context.Background()
			if err := st.SetPolicy(ctx, "web", c.policy); err != nil {
				t.Fatal(err)
			}
			err := svc.BeforeDeploy(ctx, Input{App: "web", AttemptID: "att-1", Root: t.TempDir()})
			var blocked *BlockedError
			if got := errors.As(err, &blocked); got != c.wantBlocked {
				t.Fatalf("blocked=%v err=%v", got, err)
			}
			run, gerr := st.GetRun(ctx, "att-1")
			if !c.wantStored {
				if !errors.Is(gerr, ErrRunNotFound) {
					t.Fatalf("expected no run, got %+v %v", run, gerr)
				}
				return
			}
			if gerr != nil || run.Action != c.wantAction {
				t.Fatalf("run=%+v err=%v", run, gerr)
			}
			if c.wantBlocked && !strings.Contains(blocked.Reason, "github-token at a.env:4") && run.Status != StatusError {
				t.Fatalf("reason lacks location: %q", blocked.Reason)
			}
		})
	}
}

func TestBeforeDeployDefaultsToWarn(t *testing.T) {
	svc, st := newService(t, findingScan())
	if err := svc.BeforeDeploy(context.Background(), Input{App: "web", AttemptID: "a", Root: t.TempDir()}); err != nil {
		t.Fatalf("default policy must not block: %v", err)
	}
	run, err := st.GetRun(context.Background(), "a")
	if err != nil || run.Policy != PolicyWarn || run.Action != ActionWarn {
		t.Fatalf("run=%+v err=%v", run, err)
	}
}

func TestAllowListSuppressesAndAudits(t *testing.T) {
	svc, st := newService(t, findingScan())
	ctx := context.Background()
	if err := st.SetPolicy(ctx, "web", PolicyBlock); err != nil {
		t.Fatal(err)
	}
	if err := svc.BeforeDeploy(ctx, Input{App: "web", Root: t.TempDir()}); err == nil {
		t.Fatal("expected a block before allow-listing")
	}
	if _, err := svc.Allow(ctx, "web", fp, "  ", "alice"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reason must be required, got %v", err)
	}
	if _, err := svc.Allow(ctx, "web", "not-hex", "test key", "alice"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("fingerprint must be validated, got %v", err)
	}
	if _, err := svc.Allow(ctx, "web", fp, "public test key", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BeforeDeploy(ctx, Input{App: "web", AttemptID: "a2", Root: t.TempDir()}); err != nil {
		t.Fatalf("allow-listed finding must not block: %v", err)
	}
	run, _ := st.GetRun(ctx, "a2")
	if len(run.Findings) != 1 || !run.Findings[0].Allowed || run.Action != ActionAllow {
		t.Fatalf("run=%+v", run)
	}
	list, _ := svc.AllowList(ctx, "web")
	if len(list) != 1 || list[0].Actor != "alice" || list[0].Reason != "public test key" {
		t.Fatalf("allow list %+v", list)
	}
	if ok, _ := svc.Unallow(ctx, "web", fp); !ok {
		t.Fatal("unallow reported missing")
	}
	if err := svc.BeforeDeploy(ctx, Input{App: "web", Root: t.TempDir()}); err == nil {
		t.Fatal("block must return after unallow")
	}
}

func TestBeforeDeployScansEnvWithoutRoot(t *testing.T) {
	svc, st := newService(t, nil)
	ctx := context.Background()
	if err := st.SetPolicy(ctx, "web", PolicyBlock); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"STRIPE": "sk" + "_live_" + strings.Repeat("a1B2", 6)}
	err := svc.BeforeDeploy(ctx, Input{App: "web", AttemptID: "e1", Env: env})
	var blocked *BlockedError
	if !errors.As(err, &blocked) || !strings.Contains(blocked.Reason, "stripe-secret-key at STRIPE") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(blocked.Reason, env["STRIPE"]) {
		t.Fatal("reason leaked the value")
	}
}

func TestSetPolicyValidates(t *testing.T) {
	svc, _ := newService(t, nil)
	if _, err := svc.SetPolicy(context.Background(), "web", "strict"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
	p, err := svc.SetPolicy(context.Background(), "web", "block")
	if err != nil || p != PolicyBlock {
		t.Fatalf("p=%v err=%v", p, err)
	}
}
