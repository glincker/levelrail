package moxrun

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mailbackup"
)

// cycleInit is a cut-down mail template start: quickstart once, hand both
// volumes to uid 1000 with setgid as the template does, then mox serve as root.
const cycleInit = `set -e; cd /mox
if [ ! -f config/mox.conf ]; then rm -rf /tmp/init; mkdir /tmp/init; cd /tmp/init
mox quickstart -skipdial -hostname mail.mjtest.example admin@mjtest.example 1000 </dev/null >/tmp/q 2>&1 || { tail /tmp/q; exit 1; }
cp -a config/. /mox/config/; cp -a data/. /mox/data/; cd /mox; chown -R 1000:1000 /mox/config /mox/data; chmod g+s /mox/config /mox/data; fi
exec mox serve`

type cycleLab struct {
	t   *testing.T
	ctx context.Context
	c   *docker.Client
	l   *swapLab
	id  string
}

func (m *cycleLab) sh(script string) (string, error) {
	rc, err := m.c.Exec(m.ctx, m.id, []string{"sh", "-c", script})
	if err != nil {
		return "", err
	}
	out, err := drain(rc)
	_ = rc.Close()
	return strings.TrimSpace(out), err
}

func (m *cycleLab) waitServe() {
	m.t.Helper()
	for range 60 {
		if out, _ := m.sh(`ps -o user,args | grep -q '^1000 *mox serve' && test -S /mox/data/ctl && echo up`); out == "up" {
			time.Sleep(2 * time.Second)
			return
		}
		time.Sleep(time.Second)
	}
	m.t.Fatal("mox serve did not come up")
}

// noWorkDir fails when either volume root holds a .levelrail dir.
func (m *cycleLab) noWorkDir(when string) {
	m.t.Helper()
	if out, _ := m.sh(`ls -d /mox/data/.levelrail /mox/config/.levelrail 2>/dev/null; true`); out != "" {
		m.t.Fatalf("%s: left behind %s", when, out)
	}
}

// backup runs the control plane's backup flow: the volume sweep, then mox backup, verifydata and encrypt.
func (m *cycleLab) backup(sink *mailbackup.LocalSink, rcpt age.Recipient) mailbackup.Manifest {
	m.t.Helper()
	for _, r := range []string{DataDir, ConfigDir} {
		if err := Sweep(m.ctx, m.c, m.id, r, "120"); err != nil {
			m.t.Fatalf("sweep %s: %v", r, err)
		}
	}
	mf, err := mailbackup.Backup(m.ctx, mailbackup.BackupSpec{
		Runner: ExecRunner{RT: m.c, ContainerID: m.id}, Sink: sink, StagingHost: m.t.TempDir(), StagingContainer: StagingDir,
		Recipients: []age.Recipient{rcpt},
		Fetch: func(ctx context.Context, cont, host string) error {
			return Fetch(ctx, m.c, m.id, cont, host, sink.CapBytes)
		},
		FetchConfig: func(ctx context.Context, host string) error {
			return Fetch(ctx, m.c, m.id, ConfigDir, host, sink.CapBytes)
		},
		Cleanup: func(ctx context.Context, cont string) { _ = RemoveStaging(ctx, m.c, m.id, cont) },
	})
	if err != nil || !mf.Verified {
		out, _ := m.sh(`cd /mox && mox backup ` + StagingDir + `/why 2>&1 | grep -v '^warning' | tail -n 5; ls -lanR /mox/data/.levelrail /mox/config/.levelrail 2>&1 | head -n 20`)
		m.t.Fatalf("backup: %v %+v\n%s", err, mf, out)
	}
	if out, _ := m.sh(`ls -A ` + StagingDir); out != "" {
		m.t.Fatalf("staging left behind: %s", out)
	}
	return mf
}

// TestLiveMailCycle reproduces the real-host failure (a root-owned 0700
// .levelrail in both volume roots made every mox backup fail) and runs the
// whole cycle on a real mox: recovery with nothing pending, backup, list,
// verify, dry run, a staged restore swapped in by the helper, and a backup after it.
func TestLiveMailCycle(t *testing.T) {
	img := os.Getenv("MOXRUN_LIVE_SWAP_IMAGE")
	if img == "" {
		t.Skip("set MOXRUN_LIVE_SWAP_IMAGE to the pinned mox image (internal/catalog/templates_mail.go)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	c, err := docker.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	sfx := strconv.FormatInt(time.Now().UnixNano()%1e9, 36)
	l := &swapLab{t: t, img: img, name: "mjtest-cycle-" + sfx, data: "mjtest-cycle-data-" + sfx, cfg: "mjtest-cycle-config-" + sfx}
	t.Cleanup(func() {
		_, _ = l.docker("", "rm", "-f", l.name, HelperName("mjtest-cycle-"+sfx))
		_, _ = l.docker("", "volume", "rm", "-f", l.data, l.cfg)
	})
	l.must("", "run", "-d", "--name", l.name, "-v", l.data+":"+DataDir, "-v", l.cfg+":"+ConfigDir, "--entrypoint", "sh", img, "-c", cycleInit)
	st, err := c.InspectByName(ctx, l.name)
	if err != nil || st == nil {
		t.Fatalf("inspect: %v", err)
	}
	m := &cycleLab{t: t, ctx: ctx, c: c, l: l, id: st.ID}
	m.waitServe()

	// Start up recovery with nothing pending runs state, then commit on each root.
	for _, r := range []string{DataDir, ConfigDir} {
		out, err := m.sh(`sh -c '` + strings.ReplaceAll(swapScript, `'`, `'\''`) + `' swap state ` + r)
		if err != nil || out != "none" {
			t.Fatalf("state %s: %q %v", r, out, err)
		}
		if _, err := m.sh(`sh -c '` + strings.ReplaceAll(swapScript, `'`, `'\''`) + `' swap commit ` + r + ` "" "" 2`); err != nil {
			t.Fatalf("commit %s: %v", r, err)
		}
	}
	m.noWorkDir("recovery with nothing pending")
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	sink := &mailbackup.LocalSink{Dir: t.TempDir(), CapBytes: 256 << 20}
	m.backup(sink, id.Recipient())
	m.noWorkDir("backup")

	// What older releases left: root-owned 0700 work dirs, one holding a pre-restore copy.
	if _, err := m.sh(`set -e; for r in /mox/data /mox/config; do mkdir -p $r/.levelrail; chmod 700 $r/.levelrail; done
mkdir -p /mox/data/.levelrail/pre-restore-old/accounts; chmod 700 /mox/data/.levelrail/pre-restore-old; echo x > /mox/data/.levelrail/pre-restore-old/accounts/f; chown 1000 /mox/data/.levelrail/pre-restore-old/accounts/f`); err != nil {
		t.Fatal(err)
	}
	if out, err := m.sh(`cd /mox && mox backup ` + StagingDir + `/old 2>&1`); err == nil || !strings.Contains(out, "permission denied") {
		t.Fatalf("the old layout must reproduce the failure, got %v %s", err, out)
	}
	_ = RemoveStaging(ctx, c, st.ID, StagingDir+"/old")
	time.Sleep(time.Second) // backup ids have one second resolution
	first := m.backup(sink, id.Recipient())
	if out, _ := m.sh(`ls -d /mox/config/.levelrail 2>/dev/null; stat -c %u /mox/data/.levelrail /mox/data/.levelrail/pre-restore-old`); out != "1000\n1000" {
		t.Fatalf("sweep must drop the empty work dir and hand the rest to the mox user, got %q", out)
	}
	_, _ = m.sh(`rm -rf /mox/data/.levelrail`)

	ms, err := mailbackup.List(ctx, sink)
	if err != nil || len(ms) != 2 {
		t.Fatalf("list: %d %v", len(ms), err)
	}
	spec := mailbackup.RestoreSpec{Runner: ExecRunner{RT: c, ContainerID: st.ID}, Sink: sink, ID: first.ID, Identities: []age.Identity{id}}
	if _, err := mailbackup.Check(ctx, spec); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if rep, err := mailbackup.Restore(ctx, spec, true); err != nil || rep.Files != first.Files {
		t.Fatalf("dry run: %+v %v", rep, err)
	}
	m.noWorkDir("list, verify and dry run")

	spec.DestDir = t.TempDir() + "/restore"
	if _, err := mailbackup.Restore(ctx, spec, false); err != nil {
		t.Fatalf("extract: %v", err)
	}
	roots := []Root{{Dir: DataDir, Sub: "data"}, {Dir: ConfigDir, Sub: "config"}}
	for _, r := range roots {
		if err := Push(ctx, c, c, st.ID, spec.DestDir+"/"+r.Sub, StageDir(r.Dir, first.ID)); err != nil {
			t.Fatalf("stage %s: %v", r.Dir, err)
		}
	}
	if _, err := spec.Runner.Run(ctx, "verifydata", StageDir(DataDir, first.ID)+"/data"); err != nil {
		t.Fatalf("verifydata on the staged restore: %v", err)
	}
	if err := c.Stop(ctx, st.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	h, err := StartHelper(ctx, c, img, "mjtest-cycle-"+sfx, Volumes{Data: l.data, Config: l.cfg}, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pre := PreRestoreName(first.ID, time.Now())
	for _, r := range roots {
		if out, err := h.Run(ctx, "swap", r.Dir, first.ID, pre, r.Sub); err != nil || out != "swapped" {
			t.Fatalf("swap %s: %q %v", r.Dir, out, err)
		}
	}
	if err := c.Start(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	m.waitServe()
	for _, r := range roots {
		if _, err := h.Run(ctx, "commit", r.Dir, "", "", "2"); err != nil {
			t.Fatalf("commit %s: %v", r.Dir, err)
		}
	}
	h.Close()
	if out, _ := m.sh(`stat -c %u:%a /mox/data/.levelrail /mox/config/.levelrail; ls /mox/data/.levelrail`); out != "1000:700\n1000:700\n"+pre {
		t.Fatalf("after the restore the work dirs must hold only the pre-restore copy, owned by the mox user: %q", out)
	}
	time.Sleep(time.Second)
	if out, _ := m.sh(`ls /mox/data/moxversion 2>/dev/null; true`); out != "" {
		t.Fatal("the restore must not put the backup's moxversion in the live data dir")
	}
	after := m.backup(sink, id.Recipient())
	t.Logf("backups %s (%d files), %s after restore (%d files)", first.ID, first.Files, after.ID, after.Files)
}
