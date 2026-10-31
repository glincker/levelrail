package moxrun

import (
	"context"
	"os"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mailbackup"
)

// TestLiveMoxBackupVerifyRestore runs against a real mox container named by
// MOXRUN_LIVE_CONTAINER: backup, verify, then a dry-run and a staged restore.
func TestLiveMoxBackupVerifyRestore(t *testing.T) {
	name := os.Getenv("MOXRUN_LIVE_CONTAINER")
	if name == "" {
		t.Skip("set MOXRUN_LIVE_CONTAINER to a running mox container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := docker.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.InspectByName(ctx, name)
	if err != nil || st == nil || !st.Running {
		t.Fatalf("container %s not running: %v", name, err)
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	sink := &mailbackup.LocalSink{Dir: t.TempDir(), CapBytes: 256 << 20}
	run := ExecRunner{RT: c, ContainerID: st.ID}
	var steps []string
	m, err := mailbackup.Backup(ctx, mailbackup.BackupSpec{
		Runner: run, Sink: sink, StagingHost: t.TempDir(), StagingContainer: StagingDir,
		Recipients: []age.Recipient{id.Recipient()},
		Fetch: func(ctx context.Context, cont, host string) error {
			return Fetch(ctx, c, st.ID, cont, host, sink.CapBytes)
		},
		Cleanup:  func(ctx context.Context, cont string) { _ = RemoveStaging(ctx, c, st.ID, cont) },
		Progress: func(s string) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatalf("backup: %v (steps %v)", err, steps)
	}
	t.Logf("backup %s mox %s files %d bytes %d steps %v", m.ID, m.MoxVersion, m.Files, m.Bytes, steps)
	if !m.Verified || m.Files == 0 {
		t.Fatalf("manifest not verified or empty: %+v", m)
	}
	if out, err := run.Run(ctx, "version"); err != nil || out == "" {
		t.Fatalf("staging cleanup check: %v", err)
	}
	if rc, err := c.Exec(ctx, st.ID, []string{"test", "!", "-e", StagingDir + "/" + m.ID}); err == nil {
		if _, err := rc.Read(make([]byte, 1)); err != nil && err.Error() != "EOF" {
			t.Fatalf("staging dir left behind: %v", err)
		}
		_ = rc.Close()
	}

	spec := mailbackup.RestoreSpec{Runner: run, Sink: sink, ID: m.ID, Identities: []age.Identity{id}}
	rep, err := mailbackup.Restore(ctx, spec, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	t.Logf("dry run files %d bytes %d", rep.Files, rep.Bytes)
	if rep.Files != m.Files {
		t.Fatalf("dry run saw %d files, manifest says %d", rep.Files, m.Files)
	}

	dest := t.TempDir() + "/restore"
	spec.DestDir = dest
	if _, err := mailbackup.Restore(ctx, spec, false); err != nil {
		t.Fatalf("extract: %v", err)
	}
	stage := StageDir(DataDir, m.ID)
	if err := Push(ctx, c, c, st.ID, dest+"/data", stage); err != nil {
		t.Fatalf("push: %v", err)
	}
	if _, err := run.Run(ctx, "verifydata", stage+"/data"); err != nil {
		t.Fatalf("verifydata on staged restore: %v", err)
	}
	vol := os.Getenv("MOXRUN_LIVE_DATA_VOLUME")
	if vol == "" {
		_ = RemoveStaging(ctx, c, st.ID, stage)
		return
	}
	if err := c.Stop(ctx, st.ID, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	h, err := StartHelper(ctx, c, st.Image, "mjtest", Volumes{Data: vol}, 10*time.Minute)
	if err != nil {
		_ = c.Start(ctx, st.ID)
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := h.Run(ctx, "swap", DataDir, m.ID, PreRestoreName(m.ID, time.Now()), "data"); err != nil {
		_, _ = h.Run(ctx, "rollback", DataDir)
		_ = c.Start(ctx, st.ID)
		t.Fatalf("swap: %v", err)
	}
	if err := c.Start(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	if err := WaitRunning(ctx, c, name, time.Minute, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	if out, err := run.Run(ctx, "config", "account", "add", "mjafter", "mjafter@mjtest.example"); err != nil {
		t.Fatalf("mox ctl after swap: %v %s", err, out)
	}
}
