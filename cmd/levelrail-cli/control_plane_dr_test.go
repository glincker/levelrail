package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type drFake struct {
	mu       sync.Mutex
	st       apiclient.ControlPlaneDR
	polls    int
	recips   []string
	escrowTo []string
	acked    bool
}

func newDRServer(t *testing.T, f *drFake) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	mux.HandleFunc("GET /api/v1/system/control-plane-dr", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.st.BackupRunning || f.st.DrillRunning {
			if f.polls++; f.polls >= 2 {
				f.st.BackupRunning, f.st.DrillRunning = false, false
			}
		}
		write(w, f.st)
	})
	mux.HandleFunc("PUT /api/v1/system/control-plane-dr/settings", func(w http.ResponseWriter, r *http.Request) {
		var s apiclient.ControlPlaneDRSettings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			t.Error(err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.st.Enabled, f.st.TargetID, f.st.Recipients, f.st.Schedule, f.st.RetainDaily = s.Enabled, s.TargetID, s.Recipients, s.Schedule, s.RetainDaily
		f.st.RetainWeekly, f.st.EscrowTargetID = s.RetainWeekly, s.EscrowTargetID
		write(w, f.st)
	})
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/run", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.st.BackupRunning, f.polls = true, 0
		w.WriteHeader(http.StatusAccepted)
		write(w, map[string]bool{"started": true})
	})
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/drill", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.st.DrillRunning, f.polls = true, 0
		w.WriteHeader(http.StatusAccepted)
		write(w, map[string]bool{"started": true})
	})
	mux.HandleFunc("GET /api/v1/system/control-plane-dr/backups", func(w http.ResponseWriter, _ *http.Request) {
		write(w, []apiclient.ControlPlaneOffboxBackup{{Key: "cp-backups/i/2026/09/25/a.db.age", Complete: true, SizeBytes: 12}})
	})
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/escrow", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Recipients []string `json:"recipients"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		rec, err := age.ParseX25519Recipient(body.Recipients[0])
		if err != nil {
			t.Error(err)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		var buf bytes.Buffer
		aw := armor.NewWriter(&buf)
		enc, _ := age.Encrypt(aw, rec)
		_, _ = enc.Write([]byte(`{"master_key":"AGE-SECRET-KEY-PQ-1FAKEFORTEST"}`))
		_ = enc.Close()
		_ = aw.Close()
		write(w, apiclient.ControlPlaneEscrowBundle{Armored: buf.String(), Instructions: "keep it safe", Fingerprint: "ffff", RecipientCount: 1})
	})
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/escrow/ack", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.acked = true
		write(w, f.st)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func withFastPolling(t *testing.T) {
	t.Helper()
	old := drPollInterval
	drPollInterval = time.Millisecond
	t.Cleanup(func() { drPollInterval = old })
}

func TestCLI_ControlPlaneDR_ScheduleShowAndSet(t *testing.T) {
	f := &drFake{st: apiclient.ControlPlaneDR{Schedule: "0 2 * * *", RetainDaily: 7, RetainWeekly: 4, Recipients: []string{"age1old"}, Warnings: []apiclient.ControlPlaneDRWarning{{Code: "x", Message: "add a recipient"}}}}
	srv := newDRServer(t, f)

	stdout, _ := runCLIExpectOK(t, []string{"control-plane-backups", "schedule", "show", "--api-url", srv.URL})
	if !strings.Contains(stdout, "0 2 * * *") || !strings.Contains(stdout, "warning: add a recipient") {
		t.Errorf("show = %q", stdout)
	}

	runCLIExpectOK(t, []string{"control-plane-backups", "schedule", "set", "--api-url", srv.URL, "--enable", "--destination", "t1", "--recipient", "age1a", "--recipient", "age1b", "--retain-daily", "3"})
	if !f.st.Enabled || f.st.TargetID != "t1" || len(f.st.Recipients) != 2 || f.st.RetainDaily != 3 || f.st.Schedule != "0 2 * * *" || f.st.RetainWeekly != 4 {
		t.Fatalf("settings after set = %+v", f.st)
	}
	runCLIExpectOK(t, []string{"control-plane-backups", "schedule", "set", "--api-url", srv.URL, "--disable"})
	if f.st.Enabled || f.st.TargetID != "t1" {
		t.Fatalf("untouched fields must survive: %+v", f.st)
	}
	var out, errOut strings.Builder
	if got := run("levelrail-cli-test", []string{"control-plane-backups", "schedule", "set", "--api-url", srv.URL, "--enable", "--disable"}, &out, &errOut, envMap()); got != exitValidation {
		t.Fatalf("enable+disable exit = %d", got)
	}
}

func TestCLI_ControlPlaneDR_RunNowAndDrill(t *testing.T) {
	withFastPolling(t)
	f := &drFake{st: apiclient.ControlPlaneDR{LastBackupAt: "2026-09-25T02:00:00Z"}}
	srv := newDRServer(t, f)

	stdout, _ := runCLIExpectOK(t, []string{"control-plane-backups", "run-now", "--api-url", srv.URL})
	if !strings.Contains(stdout, "2026-09-25T02:00:00Z") {
		t.Errorf("run-now = %q", stdout)
	}

	f.st.LastDrill = apiclient.ControlPlaneDRDrill{At: "2026-09-25T05:00:00Z", OK: true, Partial: true}
	stdout, _ = runCLIExpectOK(t, []string{"control-plane-backups", "drill", "run", "--api-url", srv.URL})
	if !strings.Contains(stdout, "partial") {
		t.Errorf("drill run = %q", stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"control-plane-backups", "drill", "status", "--api-url", srv.URL})
	if !strings.Contains(stdout, "passed (partial") {
		t.Errorf("drill status = %q", stdout)
	}

	f.st.LastDrill = apiclient.ControlPlaneDRDrill{At: "2026-09-25T05:00:00Z", OK: false, Detail: "checksum mismatch"}
	var out, errOut strings.Builder
	if got := run("levelrail-cli-test", []string{"control-plane-backups", "drill", "status", "--api-url", srv.URL}, &out, &errOut, envMap()); got != exitCheckFailed {
		t.Fatalf("failed drill exit = %d, want %d", got, exitCheckFailed)
	}

	f.st.LastBackupError = "denied"
	out.Reset()
	if got := run("levelrail-cli-test", []string{"control-plane-backups", "run-now", "--api-url", srv.URL}, &out, &errOut, envMap()); got != exitCheckFailed {
		t.Fatalf("failed backup exit = %d, want %d", got, exitCheckFailed)
	}
}

func TestCLI_ControlPlaneDR_ListOffbox(t *testing.T) {
	srv := newDRServer(t, &drFake{})
	stdout, _ := runCLIExpectOK(t, []string{"control-plane-backups", "list", "--offbox", "--api-url", srv.URL})
	if !strings.Contains(stdout, "cp-backups/i/2026/09/25/a.db.age") {
		t.Errorf("list --offbox = %q", stdout)
	}
}

func TestCLI_ControlPlaneDR_KeysGenerateAndEscrowRoundTrip(t *testing.T) {
	dir := t.TempDir()
	idFile := filepath.Join(dir, "identity.txt")
	stdout, stderr := runCLIExpectOK(t, []string{"control-plane-backups", "keys", "generate", "--out", idFile})
	recipient := strings.TrimSpace(stdout)
	if !strings.HasPrefix(recipient, "age1") || !strings.Contains(stderr, "OFFLINE") {
		t.Fatalf("generate stdout = %q stderr = %q", stdout, stderr)
	}
	info, err := os.Stat(idFile)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("identity file mode = %v, err = %v", info, err)
	}
	if strings.Contains(stdout, "AGE-SECRET-KEY") {
		t.Fatal("private key printed to stdout")
	}
	var out, errOut strings.Builder
	if got := run("levelrail-cli-test", []string{"control-plane-backups", "keys", "generate", "--out", idFile}, &out, &errOut, envMap()); got == exitOK {
		t.Fatal("generate must never overwrite an existing identity")
	}
	if hybridOut, _ := runCLIExpectOK(t, []string{"control-plane-backups", "keys", "generate", "--hybrid", "--out", filepath.Join(dir, "h.txt")}); !strings.HasPrefix(hybridOut, "age1pq") {
		t.Fatalf("hybrid recipient = %q", hybridOut)
	}

	f := &drFake{}
	srv := newDRServer(t, f)
	bundle := filepath.Join(dir, "escrow.age")
	_, stderr = runCLIExpectOK(t, []string{"control-plane-backups", "escrow", "--api-url", srv.URL, "--recipient", recipient, "--out", bundle, "--ack"})
	if !strings.Contains(stderr, "NOT in the same place") || !f.acked {
		t.Fatalf("escrow stderr = %q acked = %v", stderr, f.acked)
	}
	if info, err := os.Stat(bundle); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("bundle mode = %v, err = %v", info, err)
	}
	if _, err := os.Stat(bundle + ".instructions.txt"); err != nil {
		t.Fatal("instructions file missing")
	}
	if got := run("levelrail-cli-test", []string{"control-plane-backups", "escrow", "--api-url", srv.URL, "--recipient", recipient, "--out", bundle}, &out, &errOut, envMap()); got == exitOK {
		t.Fatal("escrow must never overwrite an existing bundle")
	}

	stdout, _ = runCLIExpectOK(t, []string{"control-plane-backups", "escrow", "open", bundle, "--identity", idFile})
	if strings.TrimSpace(stdout) != "AGE-SECRET-KEY-PQ-1FAKEFORTEST" {
		t.Fatalf("opened key = %q", stdout)
	}

	otherID := filepath.Join(dir, "other.txt")
	runCLIExpectOK(t, []string{"control-plane-backups", "keys", "generate", "--out", otherID})
	out.Reset()
	if got := run("levelrail-cli-test", []string{"control-plane-backups", "escrow", "open", bundle, "--identity", otherID}, &out, &errOut, envMap()); got != exitValidation {
		t.Fatalf("wrong identity exit = %d", got)
	}
}
