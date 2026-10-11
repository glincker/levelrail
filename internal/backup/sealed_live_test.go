package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// liveS3 starts a throwaway S3 compatible server and returns its endpoint
// with an existing bucket. LEVELRAIL_LIVE_S3_IMAGE and
// LEVELRAIL_LIVE_S3_ARGS select the image (MinIO: "minio/minio" and
// "server /data"); the default is SeaweedFS, which needs no credentials.
func liveS3(t *testing.T) Destination {
	t.Helper()
	image := os.Getenv("LEVELRAIL_LIVE_S3_IMAGE")
	args := os.Getenv("LEVELRAIL_LIVE_S3_ARGS")
	port := "8333"
	env := []string{}
	if image == "" {
		image, args = "chrislusf/seaweedfs:latest", "server -s3 -dir=/data"
	} else {
		port = "9000"
		env = []string{"-e", "MINIO_ROOT_USER=minioadmin", "-e", "MINIO_ROOT_PASSWORD=minioadmin123"}
	}
	name := fmt.Sprintf("backup-live-s3-%d", time.Now().UnixNano())
	runArgs := append([]string{"run", "-d", "--rm", "--name", name, "-p", "127.0.0.1::" + port}, env...)
	runArgs = append(runArgs, image)
	runArgs = append(runArgs, strings.Fields(args)...)
	if out, err := exec.Command("docker", runArgs...).CombinedOutput(); err != nil { //nolint:gosec // test helper, fixed binary
		t.Skipf("cannot start %s: %v: %s", image, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() }) //nolint:gosec // test helper

	var endpoint string
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "port", name, port+"/tcp").Output() //nolint:gosec // test helper
		if err == nil {
			hostPort := strings.TrimSpace(strings.Split(string(out), "\n")[0])
			if i := strings.LastIndex(hostPort, ":"); i >= 0 {
				endpoint = "http://127.0.0.1:" + hostPort[i+1:]
			}
		}
		if endpoint != "" {
			dest := Destination{Provider: store.BackupProviderCustom, Endpoint: endpoint, Region: "us-east-1", Bucket: "backups", AccessKeyID: "minioadmin", SecretAccessKey: "minioadmin123"}
			client, cerr := newS3Client(context.Background(), dest)
			if cerr == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, berr := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(dest.Bucket)})
				cancel()
				if berr == nil || strings.Contains(berr.Error(), "BucketAlreadyOwnedByYou") {
					return dest
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Skipf("S3 server %s did not become ready", image)
	return Destination{}
}

func runIn(ctx context.Context, t *testing.T, rt docker.Runtime, volume string, script string) string {
	t.Helper()
	id, err := createVolumeHelper(ctx, rt, volume, "sealed-live-helper", false)
	if err != nil {
		t.Fatalf("helper for %s: %v", volume, err)
	}
	defer func() { _ = rt.Remove(context.Background(), id, true) }()
	rc, err := rt.Exec(ctx, id, []string{"sh", "-c", script})
	if err != nil {
		t.Fatalf("exec %q: %v", script, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("exec %q: %v", script, err)
	}
	return string(out)
}

// TestSealedVolumeBackup_Live_BackupDestroyRestoreDrill is the exit
// criterion: back up a real volume to a real S3 server, delete it, restore
// into a new volume (as another app would get), compare content, run the
// drill, then corrupt and truncate the stored object and require the drill
// to fail loudly.
func TestSealedVolumeBackup_Live_BackupDestroyRestoreDrill(t *testing.T) {
	rt := liveRuntime(t)
	dest := liveS3(t)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000)
	src := "backup-live-src-" + suffix
	restoredAs := "app-copy-" + suffix + "-data"
	removeVolumeAfterTest(t, rt, src, restoredAs)

	if err := rt.EnsureVolume(ctx, src); err != nil {
		t.Fatal(err)
	}
	runIn(ctx, t, rt, src, `
		mkdir -p /vol/owned /vol/nested/deep &&
		echo original > /vol/file.txt &&
		echo secret > /vol/.hidden &&
		head -c 3000000 /dev/urandom > /vol/nested/deep/blob.bin &&
		echo owned > /vol/owned/f &&
		chown -R 1234:5678 /vol/owned && chmod 750 /vol/owned && chmod 640 /vol/owned/f &&
		touch -d '2020-01-02 03:04:05' /vol/owned/f &&
		ln -s file.txt /vol/link`)
	wantSums := runIn(ctx, t, rt, src, `cd /vol && find . -type f -exec sha256sum {} + | sort -k2 && stat -c '%n %u %g %a' owned owned/f && readlink link`)

	hist := newMemHistory()
	hist.target = store.BackupTarget{ID: "bkt_test", Name: "live", Provider: store.BackupProviderCustom, Endpoint: dest.Endpoint, Region: dest.Region, Bucket: dest.Bucket}
	secrets := newTestSecrets()
	sealer := newTestSealer(t)
	runner := &Runner{
		Store: hist, Secrets: secrets, Uploader: S3Uploader{},
		VolumeArchiver: &ContainerVolumeArchiver{Runtime: rt},
		Volume:         &VolumeBackupOptions{Sealer: sealer, Policies: hist, Attempts: 2},
	}
	if err := runner.RunVolumeBackup(ctx, "bkh_live", "web", "data", src, "bkt_test", ""); err != nil {
		t.Fatalf("backup: %v", err)
	}
	row := hist.rows["bkh_live"]
	if row.Status != store.BackupStatusSucceeded || row.Codec != CodecZstdAge || row.ManifestFiles < 5 {
		t.Fatalf("row = %+v", row)
	}
	info, err := S3Prober{}.Head(ctx, dest, row.ObjectKey)
	if err != nil || !info.Exists || info.Size != row.SizeBytes {
		t.Fatalf("stored object: %+v err %v, recorded size %d", info, err, row.SizeBytes)
	}

	if err := rt.RemoveVolume(ctx, src); err != nil {
		t.Fatalf("destroy source volume: %v", err)
	}

	// Restore as a new app's volume through the same code path the API uses.
	clone := &VolumeCloneRestoreRunner{
		Store: hist, Secrets: secrets, Downloader: S3Downloader{},
		VolumeRestorer: &ContainerVolumeRestorer{Runtime: rt}, Volumes: rt, Identities: sealer.Identities,
	}
	if err := clone.RunVolumeCloneRestore(ctx, "vcr_1", "web", "data", restoredAs, "bkh_live"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	gotSums := runIn(ctx, t, rt, restoredAs, `cd /vol && find . -type f -exec sha256sum {} + | sort -k2 && stat -c '%n %u %g %a' owned owned/f && readlink link`)
	if gotSums != wantSums {
		t.Fatalf("restored content differs\nwant:\n%s\ngot:\n%s", wantSums, gotSums)
	}
	if mt := strings.TrimSpace(runIn(ctx, t, rt, restoredAs, `stat -c %Y /vol/owned/f`)); mt != "1577934245" {
		t.Fatalf("mtime = %s, want 1577934245", mt)
	}
	restoredArchive, err := (&ContainerVolumeArchiver{Runtime: rt}).Archive(ctx, restoredAs)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ComputeManifest(restoredArchive)
	_ = restoredArchive.Close()
	if err != nil || m.Digest != row.ManifestDigest {
		t.Fatalf("restored volume manifest %q != backed up manifest %q (err %v)", m.Digest, row.ManifestDigest, err)
	}

	drill := &DrillRunner{
		Store: hist, Secrets: secrets, Downloader: S3Downloader{}, Prober: S3Prober{}, Identities: sealer.Identities,
		Restorer: &ContainerVolumeRestorer{Runtime: rt}, Archiver: &ContainerVolumeArchiver{Runtime: rt}, Volumes: rt, Remover: rt,
	}
	d, err := drill.RunDrill(ctx, "bkd_live1", "bkh_live", store.DrillTriggerManual)
	if err != nil || d.Status != store.DrillStatusPassed || !d.ContentOK {
		t.Fatalf("drill on healthy backup: %v %+v", err, d)
	}
	if vols, _ := rt.ListVolumesByPrefix(ctx, "drill-bkd_live1"); len(vols) != 0 {
		t.Fatalf("scratch volume left behind: %+v", vols)
	}

	object, err := io.ReadAll(mustDownload(t, S3Downloader{}, dest, row.ObjectKey))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		mutate  func([]byte) []byte
		wantErr string
	}{
		{"corrupted object", func(b []byte) []byte { c := append([]byte(nil), b...); c[len(c)/2] ^= 0xff; return c }, ""},
		{"truncated object", func(b []byte) []byte { return b[:len(b)/2] }, "truncated"},
	}
	for i, c := range cases {
		if err := (S3Uploader{}).Upload(ctx, dest, row.ObjectKey, bytes.NewReader(c.mutate(object)), -1); err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("bkd_live%d", i+2)
		d, err := drill.RunDrill(ctx, id, "bkh_live", store.DrillTriggerScheduled)
		if err == nil || d.Status != store.DrillStatusFailed {
			t.Fatalf("%s: drill must fail loudly, got status %q err %v", c.name, d.Status, err)
		}
		if c.wantErr != "" && !strings.Contains(d.Error, c.wantErr) {
			t.Fatalf("%s: error %q should mention %q", c.name, d.Error, c.wantErr)
		}
		if vols, _ := rt.ListVolumesByPrefix(ctx, "drill-"+id); len(vols) != 0 {
			t.Fatalf("%s: scratch volume left behind", c.name)
		}
	}

	// An in-place restore of the damaged object must also refuse before
	// touching anything.
	guard := "app-guard-" + suffix
	removeVolumeAfterTest(t, rt, guard)
	if err := rt.EnsureVolume(ctx, guard); err != nil {
		t.Fatal(err)
	}
	runIn(ctx, t, rt, guard, "echo keep > /vol/keep")
	err = downloadAndRestoreVolume(ctx, hist, secrets, S3Downloader{}, &ContainerVolumeRestorer{Runtime: rt}, sealer.Identities, guard, "bkh_live")
	if err == nil {
		t.Fatal("restore of a truncated object must fail")
	}
	if got := strings.TrimSpace(runIn(ctx, t, rt, guard, "cat /vol/keep")); got != "keep" {
		t.Fatalf("a refused restore modified the target volume: %q", got)
	}
}

func mustDownload(t *testing.T, d Downloader, dest Destination, key string) io.ReadCloser {
	t.Helper()
	rc, err := d.Download(context.Background(), dest, key)
	if err != nil {
		t.Fatal(err)
	}
	return rc
}

// TestSealedVolumeBackup_Live_HooksAndPause runs the quiesce path against a
// real container: the pre-hook runs inside it, a failing hook fails the
// backup, and the container is not left paused.
func TestSealedVolumeBackup_Live_HooksAndPause(t *testing.T) {
	rt := liveRuntime(t)
	dest := liveS3(t)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000)
	vol := "backup-live-pause-" + suffix
	ctrName := "backup-live-app-" + suffix
	removeVolumeAfterTest(t, rt, vol)
	if err := rt.EnsureVolume(ctx, vol); err != nil {
		t.Fatal(err)
	}
	id, err := rt.Create(ctx, docker.ContainerSpec{Name: ctrName, Image: volumeHelperImage, Command: []string{"sleep", "600"},
		Volumes: []docker.VolumeMount{{Name: vol, ContainerPath: "/data"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Remove(context.Background(), id, true) })
	if err := rt.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	if out, err := rt.Exec(ctx, id, []string{"sh", "-c", "echo data > /data/file"}); err == nil {
		_, _ = io.Copy(io.Discard, out)
		_ = out.Close()
	}

	hist := newMemHistory()
	hist.target = store.BackupTarget{ID: "bkt_test", Provider: store.BackupProviderCustom, Endpoint: dest.Endpoint, Region: dest.Region, Bucket: dest.Bucket}
	hist.policies["web/data"] = store.VolumeBackupPolicy{PreHook: "echo ran > /tmp/pre-hook", PostHook: "echo ran > /tmp/post-hook", Quiesce: store.VolumeQuiescePause}
	runner := &Runner{
		Store: hist, Secrets: newTestSecrets(), Uploader: S3Uploader{}, VolumeArchiver: &ContainerVolumeArchiver{Runtime: rt},
		Volume: &VolumeBackupOptions{
			Sealer: &Sealer{}, Policies: hist,
			Quiesce: &ContainerQuiescer{Resolve: func(context.Context, string) (docker.Runtime, string, error) { return rt, id, nil }},
		},
	}
	if err := runner.RunVolumeBackup(ctx, "bkh_pause", "web", "data", vol, "bkt_test", ""); err != nil {
		t.Fatalf("backup with hooks: %v", err)
	}
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Paused}}", id).Output() //nolint:gosec // test helper
	if err != nil || strings.TrimSpace(string(out)) != "false" {
		t.Fatalf("container left paused: %q %v", out, err)
	}
	for _, f := range []string{"/tmp/pre-hook", "/tmp/post-hook"} {
		if got, err := exec.Command("docker", "exec", id, "cat", f).Output(); err != nil || strings.TrimSpace(string(got)) != "ran" { //nolint:gosec // test helper
			t.Fatalf("hook file %s: %q %v", f, got, err)
		}
	}

	hist.policies["web/data"] = store.VolumeBackupPolicy{PreHook: "exit 3"}
	err = runner.RunVolumeBackup(ctx, "bkh_hookfail", "web", "data", vol, "bkt_test", "")
	if err == nil || !strings.Contains(err.Error(), "pre-backup hook") {
		t.Fatalf("failing pre-hook must fail the backup, got %v", err)
	}
	if hist.rows["bkh_hookfail"].Status != store.BackupStatusFailed {
		t.Fatalf("status = %q", hist.rows["bkh_hookfail"].Status)
	}
}

// TestDatabaseDrill_Live_Postgres drills a real pg_dump: restored into a
// scratch Postgres container built from the same image, validated with a
// query, destroyed. A tampered dump must fail the drill.
func TestDatabaseDrill_Live_Postgres(t *testing.T) {
	rt := liveRuntime(t)
	dest := liveS3(t)
	ctx := context.Background()

	const dbName = "drilldb"
	image := "postgres:16-alpine"
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000)
	srcName := "backup-live-pg-" + suffix
	id, err := rt.Create(ctx, docker.ContainerSpec{Name: srcName, Image: image, Env: scratchEnv(store.EnginePostgres, dbName)})
	if err != nil {
		t.Skipf("cannot create %s: %v", image, err)
	}
	t.Cleanup(func() { _ = rt.Remove(context.Background(), id, true) })
	if err := rt.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	restorer := &ContainerRestorer{Runtime: rt, ReadyTimeout: 90 * time.Second}
	if err := restorer.waitEngineReady(ctx, store.EnginePostgres, srcName); err != nil {
		t.Fatal(err)
	}
	seed := `psql -U drilldb -d drilldb -c "create table notes(id int primary key, body text); insert into notes select g, 'row ' || g from generate_series(1,50) g; create table tags(t text);"`
	out, err := rt.Exec(ctx, srcName, []string{"sh", "-c", seed})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, out)
	_ = out.Close()

	hist := newMemHistory()
	hist.target = store.BackupTarget{ID: "bkt_test", Provider: store.BackupProviderCustom, Endpoint: dest.Endpoint, Region: dest.Region, Bucket: dest.Bucket}
	runner := &Runner{Store: hist, Secrets: newTestSecrets(), Dumper: &ContainerDumper{Runtime: rt}, Uploader: S3Uploader{}}
	if err := runner.RunBackup(ctx, "bkh_pg", dbName, store.EnginePostgres, srcName, "bkt_test"); err != nil {
		t.Fatalf("backup: %v", err)
	}

	drill := &DrillRunner{
		Store: hist, Secrets: newTestSecrets(), Downloader: S3Downloader{}, Prober: S3Prober{},
		Runtime: rt, DBRestorer: restorer,
		DBInfo:   func(context.Context, string) (string, string, error) { return store.EnginePostgres, "16", nil },
		ImageFor: func(string, string) string { return image },
	}
	d, err := drill.RunDrill(ctx, "bkd_pg1", "bkh_pg", store.DrillTriggerManual)
	if err != nil || d.Status != store.DrillStatusPassed || !d.RestoreOK || !d.ContentOK || d.Files != 2 {
		t.Fatalf("postgres drill: %v %+v", err, d)
	}
	if st, _ := rt.InspectByName(ctx, "drill-bkd_pg1"); st != nil {
		t.Fatal("scratch container left behind")
	}

	object, _ := io.ReadAll(mustDownload(t, S3Downloader{}, dest, hist.rows["bkh_pg"].ObjectKey))
	broken := bytes.Replace(object, []byte("CREATE TABLE public.notes"), []byte("CREATE TABLE public.notes ("), 1)
	if err := (S3Uploader{}).Upload(ctx, dest, hist.rows["bkh_pg"].ObjectKey, bytes.NewReader(broken), -1); err != nil {
		t.Fatal(err)
	}
	d, err = drill.RunDrill(ctx, "bkd_pg2", "bkh_pg", store.DrillTriggerManual)
	if err == nil || d.Status != store.DrillStatusFailed {
		t.Fatalf("tampered dump must fail the drill: %v %+v", err, d)
	}
	if st, _ := rt.InspectByName(ctx, "drill-bkd_pg2"); st != nil {
		t.Fatal("scratch container left behind after a failed drill")
	}
}
