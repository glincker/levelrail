package supplychain

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func setSettings(t *testing.T, h *harness, app string, enabled bool, gate string) {
	t.Helper()
	if _, err := h.svc.SaveSettings(context.Background(), app, SettingsPatch{Enabled: &enabled, Gate: &gate}); err != nil {
		t.Fatalf("save settings %s: %v", app, err)
	}
}

func isBlocked(err error) bool {
	var b *BlockedError
	return errors.As(err, &b)
}

func TestAfterBuild_WithoutAttemptIDStillGatesAndPersistsNothing(t *testing.T) {
	h := newHarness(t, nil)
	setSettings(t, h, "web", true, "block_on_critical")
	if err := h.svc.AfterBuild(context.Background(), "web", "", attest(t)); !isBlocked(err) {
		t.Fatalf("a build with no attempt id must still be gated, got %v", err)
	}
	if len(h.runner.specs) != 1 {
		t.Errorf("scans = %d, want 1", len(h.runner.specs))
	}
	if recs, _ := h.svc.List(context.Background(), "web"); len(recs) != 0 || h.sbomFiles(t) != 0 {
		t.Errorf("a transient gate must not persist records or files: %d records, %d files", len(recs), h.sbomFiles(t))
	}
}

func TestAfterBuild_MultiServiceEachServiceIsGated(t *testing.T) {
	h := newHarness(t, nil)
	setSettings(t, h, "shop-web", true, "block_on_critical")
	setSettings(t, h, "shop-api", true, "block_on_critical")
	setSettings(t, h, "shop-worker", false, "off")
	if _, err := h.svc.ArmOverride(context.Background(), "shop-api", "hotfix"); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"shop-web": true, "shop-api": false, "shop-worker": false}
	for app, wantBlock := range want {
		if err := h.svc.AfterBuild(context.Background(), app, "", attest(t)); isBlocked(err) != wantBlock {
			t.Errorf("%s: blocked = %v (%v), want %v", app, isBlocked(err), err, wantBlock)
		}
	}
	if err := h.svc.AfterBuild(context.Background(), "shop-api", "", attest(t)); !isBlocked(err) {
		t.Errorf("shop-api: the override is one-shot, got %v", err)
	}
}

func TestAfterBuild_SBOMStoreFailureStillGates(t *testing.T) {
	h := newHarness(t, nil)
	setSettings(t, h, "web", true, "block_on_critical")
	if err := h.svc.AfterBuild(context.Background(), "web", "../bad", attest(t)); !isBlocked(err) {
		t.Fatalf("a failed SBOM write must not disable the gate, got %v", err)
	}
}

func TestAfterBuild_UnexpectedScannerReportIsRecordedAsFailedAndFailsOpen(t *testing.T) {
	for _, out := range []string{`{}`, `{"Results":[]}`, `[]`, ``} {
		h := newHarness(t, nil)
		setSettings(t, h, "web", true, "block_on_critical")
		h.runner.stdout = []byte(out)
		if err := h.svc.AfterBuild(context.Background(), "web", "da_1", attest(t)); err != nil {
			t.Fatalf("%q: an unexpected report fails open, got %v", out, err)
		}
		rec, _ := h.svc.Get(context.Background(), "web", "da_1")
		if rec.ScanStatus != ScanFailed || rec.ScanError == "" || rec.Scan != nil || rec.GateAction != ActionAllow {
			t.Errorf("%q: record must say the scan failed, not clean: %+v", out, rec)
		}
	}
}

func TestSaveSettings_NeverArmsOrRestoresAnOverride(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, nil)
	setSettings(t, h, "web", true, "block_on_critical")
	stale, err := h.sql.GetSettings(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.ArmOverride(ctx, "web", "outage"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatalf("override should release: %v", err)
	}
	stale.OverrideReason, stale.OverrideArmedAt = "outage", h.now
	if err := h.sql.SaveSettings(ctx, stale); err != nil {
		t.Fatal(err)
	}
	setSettings(t, h, "web", true, "block_on_critical")
	if err := h.svc.AfterBuild(ctx, "web", "da_2", attest(t)); !isBlocked(err) {
		t.Fatalf("a consumed override must stay consumed after a settings write, got %v", err)
	}
	if st, _ := h.svc.Settings(ctx, "web"); st.OverrideReason != "" || !st.OverrideArmedAt.IsZero() {
		t.Errorf("settings = %+v", st)
	}
}

func TestSaveSettings_KeepsAnArmedOverrideWhileGateStaysBlocking(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, nil)
	setSettings(t, h, "web", true, "block_on_critical")
	if _, err := h.svc.ArmOverride(ctx, "web", "outage"); err != nil {
		t.Fatal(err)
	}
	setSettings(t, h, "web", true, "block_on_critical")
	if st, _ := h.svc.Settings(ctx, "web"); st.OverrideReason != "outage" {
		t.Errorf("an unrelated settings write must not erase an armed override: %+v", st)
	}
}

func TestSaveSettings_LeavingBlockModeDisarmsTheOverride(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, nil)
	setSettings(t, h, "web", true, "block_on_critical")
	if _, err := h.svc.ArmOverride(ctx, "web", "outage"); err != nil {
		t.Fatal(err)
	}
	setSettings(t, h, "web", true, "warn")
	setSettings(t, h, "web", true, "block_on_critical")
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); !isBlocked(err) {
		t.Fatalf("switching the gate away and back must not resurrect the override, got %v", err)
	}
}

func TestArmOverride_WithoutStoredBlockGateIsRejected(t *testing.T) {
	h := newHarness(t, nil)
	if _, err := h.svc.ArmOverride(context.Background(), "web", "why"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("no settings row: %v", err)
	}
}

func TestOverride_ConcurrentReleasesAndSettingsWritesConsumeItOnce(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, nil)
	setSettings(t, h, "web", true, "block_on_critical")
	if _, err := h.svc.ArmOverride(ctx, "web", "outage"); err != nil {
		t.Fatal(err)
	}
	sbom := attest(t)
	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	passed := 0
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := h.svc.AfterBuild(ctx, "web", "", sbom); err == nil {
				mu.Lock()
				passed++
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			on, gate := true, "block_on_critical"
			_, _ = h.svc.SaveSettings(ctx, "web", SettingsPatch{Enabled: &on, Gate: &gate})
		}()
	}
	wg.Wait()
	if passed != 1 {
		t.Errorf("%d releases passed on a one-shot override, want exactly 1", passed)
	}
}
