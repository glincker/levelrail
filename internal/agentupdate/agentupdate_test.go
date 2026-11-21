package agentupdate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

const testAsset = "levelrail-agent-linux-amd64"

type fakeRelease struct {
	body      []byte
	sumBody   []byte
	verifyErr error
	fetched   []string
	gate      chan struct{}
	mu        sync.Mutex
	verified  bool
}

func (f *fakeRelease) Fetch(_ context.Context, tag, asset, dir string) (selfupgrade.Fetched, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, tag)
	f.mu.Unlock()
	if f.gate != nil {
		<-f.gate
	}
	bin := filepath.Join(dir, asset)
	sums := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(bin, f.body, 0o600); err != nil {
		return selfupgrade.Fetched{}, err
	}
	sb := f.sumBody
	if sb == nil {
		sb = f.body
	}
	line := fmt.Sprintf("%x  %s\n", sha256.Sum256(sb), asset)
	if err := os.WriteFile(sums, []byte(line), 0o600); err != nil {
		return selfupgrade.Fetched{}, err
	}
	return selfupgrade.Fetched{BinaryPath: bin, ChecksumsPath: sums, BundlePath: filepath.Join(dir, "b")}, nil
}

func (f *fakeRelease) Verify(context.Context, string, string) (string, error) {
	f.verified = true
	return "fake signature ok", f.verifyErr
}

type fakeService struct {
	restarts int
	err      error
}

func (s *fakeService) Restart(context.Context) error { s.restarts++; return s.err }

func setup(t *testing.T, rel *fakeRelease, d nodeinsight.UpdateDecision) (Options, *fakeService) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "levelrail-agent")
	if err := os.WriteFile(exe, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := &fakeService{}
	return Options{
		Decide:  func(context.Context) (nodeinsight.UpdateDecision, error) { return d, nil },
		Release: rel, Service: svc, Asset: testAsset, Current: "v1.0.0", ExePath: exe, DataDir: filepath.Dir(exe),
	}, svc
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(p))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var install = nodeinsight.UpdateDecision{Action: nodeinsight.UpdateInstall, Target: "v1.1.0", Reason: "behind"}

func TestApplySwapsKeepsPrevAndRestarts(t *testing.T) {
	rel := &fakeRelease{body: []byte("new")}
	o, svc := setup(t, rel, install)
	o.Apply = true
	res, err := Run(context.Background(), o)
	if err != nil || res.Action != ActionRestarted {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if read(t, o.ExePath) != "new" || read(t, PrevPath(o.ExePath)) != "old" || svc.restarts != 1 || !rel.verified {
		t.Fatalf("swap state wrong, restarts=%d verified=%v", svc.restarts, rel.verified)
	}
}

func TestNoApplyWithoutAutoSettingOnlyReports(t *testing.T) {
	rel := &fakeRelease{body: []byte("new")}
	o, svc := setup(t, rel, install)
	res, err := Run(context.Background(), o)
	if err != nil || res.Action != ActionAvailable || len(rel.fetched) != 0 || svc.restarts != 0 {
		t.Fatalf("res=%+v err=%v fetched=%v", res, err, rel.fetched)
	}
}

func TestAutoApplyInstallsWithoutFlag(t *testing.T) {
	d := install
	d.AutoApply = true
	o, svc := setup(t, &fakeRelease{body: []byte("new")}, d)
	res, err := Run(context.Background(), o)
	if err != nil || res.Action != ActionRestarted || svc.restarts != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestCheckNeverDownloads(t *testing.T) {
	rel := &fakeRelease{body: []byte("new")}
	o, _ := setup(t, rel, install)
	o.Check, o.Apply = true, true
	res, _ := Run(context.Background(), o)
	if res.Action != ActionAvailable || len(rel.fetched) != 0 {
		t.Fatalf("res=%+v", res)
	}
}

func TestVerificationFailuresLeaveBinaryUntouched(t *testing.T) {
	cases := map[string]*fakeRelease{
		"signature": {body: []byte("new"), verifyErr: errors.New("bad signature")},
		"checksum":  {body: []byte("new"), sumBody: []byte("other")},
	}
	for name, rel := range cases {
		t.Run(name, func(t *testing.T) {
			o, svc := setup(t, rel, install)
			o.Apply = true
			if _, err := Run(context.Background(), o); err == nil {
				t.Fatal("want error")
			}
			if read(t, o.ExePath) != "old" || svc.restarts != 0 {
				t.Fatal("binary changed or service restarted")
			}
			if _, err := os.Stat(PrevPath(o.ExePath)); err == nil {
				t.Fatal("prev must not exist")
			}
		})
	}
}

func TestSmokeVersionMismatchRefused(t *testing.T) {
	o, _ := setup(t, &fakeRelease{body: []byte("new")}, install)
	o.Apply = true
	o.Smoke = func(context.Context, string) (string, error) { return "v9.9.9", nil }
	if _, err := Run(context.Background(), o); !errors.Is(err, selfupgrade.ErrVersionMismatch) {
		t.Fatalf("err=%v", err)
	}
	if read(t, o.ExePath) != "old" {
		t.Fatal("binary changed")
	}
}

func TestUnsafeTagAndMatchingVersionIgnored(t *testing.T) {
	bad := install
	bad.Target = "v1;rm"
	o, _ := setup(t, &fakeRelease{}, bad)
	o.Apply = true
	if _, err := Run(context.Background(), o); err == nil {
		t.Fatal("want unsafe tag error")
	}
	same := install
	same.Target = "v1.0.0"
	o, _ = setup(t, &fakeRelease{}, same)
	if res, err := Run(context.Background(), o); err != nil || res.Action != ActionNone {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRestartFailureKeepsNewBinary(t *testing.T) {
	o, svc := setup(t, &fakeRelease{body: []byte("new")}, install)
	svc.err = ErrNoServiceManager
	o.Apply = true
	res, err := Run(context.Background(), o)
	if err == nil || res.Action != ActionStaged || read(t, o.ExePath) != "new" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRollbackRestoresPrev(t *testing.T) {
	o, svc := setup(t, &fakeRelease{body: []byte("new")}, install)
	o.Apply = true
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	res, err := Rollback(context.Background(), o.ExePath, o.DataDir, svc)
	if err != nil || res.Action != ActionRolled {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if read(t, o.ExePath) != "old" || read(t, o.ExePath+".failed") != "new" || svc.restarts != 2 {
		t.Fatal("rollback state wrong")
	}
	if _, err := Rollback(context.Background(), o.ExePath, o.DataDir, svc); err == nil {
		t.Fatal("second rollback has no prev")
	}
}

func TestSystemServiceArgv(t *testing.T) {
	for goos, want := range map[string]string{"linux": "systemctl", "darwin": "launchctl", "windows": "powershell"} {
		argv, err := SystemService{Name: "levelrail-agent", GOOS: goos}.Argv()
		if err != nil || argv[0] != want {
			t.Fatalf("%s: %v %v", goos, argv, err)
		}
	}
	if _, err := (SystemService{Name: "a'; x", GOOS: "windows"}).Argv(); err == nil {
		t.Fatal("unsafe name accepted")
	}
	if _, err := (SystemService{Name: "x", GOOS: "plan9"}).Argv(); !errors.Is(err, ErrNoServiceManager) {
		t.Fatal("want ErrNoServiceManager")
	}
}

func TestInstallWritesMarkerAndRollbackRecordsFailedTarget(t *testing.T) {
	o, svc := setup(t, &fakeRelease{body: []byte("new")}, install)
	o.Apply = true
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	m, ok := ReadMarker(o.DataDir)
	if !ok || m.From != "v1.0.0" || m.To != "v1.1.0" || m.InstalledAt.IsZero() {
		t.Fatalf("marker = %+v %v", m, ok)
	}
	if _, err := Rollback(context.Background(), o.ExePath, o.DataDir, svc); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadMarker(o.DataDir); ok {
		t.Fatal("marker must be cleared after rollback")
	}
	if f, ok := ReadFailed(o.DataDir); !ok || f.Target != "v1.1.0" {
		t.Fatalf("failed = %+v %v", f, ok)
	}
}

func TestFailedTargetIsNotRetriedUnlessApplied(t *testing.T) {
	rel := &fakeRelease{body: []byte("new"), verifyErr: errors.New("bad signature")}
	d := install
	d.AutoApply = true
	o, _ := setup(t, rel, d)
	if _, err := Run(context.Background(), o); err == nil {
		t.Fatal("first attempt must fail")
	}
	res, err := Run(context.Background(), o)
	if err != nil || res.Action != ActionHold || len(rel.fetched) != 1 {
		t.Fatalf("res=%+v err=%v fetched=%v", res, err, rel.fetched)
	}
	// A different target is tried; an explicit --apply retries the failed one.
	d2 := d
	d2.Target = "v1.2.0"
	o.Decide = func(context.Context) (nodeinsight.UpdateDecision, error) { return d2, nil }
	_, _ = Run(context.Background(), o)
	if len(rel.fetched) != 2 {
		t.Fatalf("new target not tried: %v", rel.fetched)
	}
	o.Decide = func(context.Context) (nodeinsight.UpdateDecision, error) { return d, nil }
	o.Apply = true
	_, _ = Run(context.Background(), o)
	if len(rel.fetched) != 3 {
		t.Fatalf("apply must retry: %v", rel.fetched)
	}
}

func TestConcurrentInstallsAreSerialized(t *testing.T) {
	rel := &fakeRelease{body: []byte("new"), gate: make(chan struct{})}
	o, _ := setup(t, rel, install)
	errc := make(chan error, 1)
	go func() { _, err := Install(context.Background(), o, "v1.1.0"); errc <- err }()
	for i := 0; i < 200; i++ {
		rel.mu.Lock()
		n := len(rel.fetched)
		rel.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := Install(context.Background(), o, "v1.1.0"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second install err = %v, want ErrBusy", err)
	}
	close(rel.gate)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestServiceNameDefaultsAndDash(t *testing.T) {
	if DefaultServiceName("darwin") != "com.levelrail.agent" || DefaultServiceName("linux") != "levelrail-agent" {
		t.Fatal("wrong per-OS service name")
	}
	if _, err := (SystemService{Name: "-evil", GOOS: "linux"}).Argv(); err == nil {
		t.Fatal("leading dash accepted")
	}
}
