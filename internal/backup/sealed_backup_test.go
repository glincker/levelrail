package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newTestSealer(t *testing.T) *Sealer {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return &Sealer{Recipients: []age.Recipient{id.Recipient()}, Identities: []age.Identity{id}}
}

func TestRetentionPolicy_ExpiredByPolicy(t *testing.T) {
	day := func(d, h int) time.Time { return time.Date(2026, 3, d, h, 0, 0, 0, time.UTC) }
	items := []RetentionItem{
		{"d5a", day(5, 20)}, {"d5b", day(5, 3)}, {"d4", day(4, 3)}, {"d3", day(3, 3)},
		{"d1", day(1, 3)}, {"feb", time.Date(2026, 2, 20, 3, 0, 0, 0, time.UTC)},
		{"jan", time.Date(2026, 1, 20, 3, 0, 0, 0, time.UTC)},
	}
	cases := []struct {
		name   string
		policy RetentionPolicy
		want   []string
	}{
		{"inactive keeps everything", RetentionPolicy{}, nil},
		{"daily 2 keeps newest of two latest days", RetentionPolicy{Daily: 2}, []string{"d5b", "d3", "d1", "feb", "jan"}},
		{"daily 1 keeps only newest", RetentionPolicy{Daily: 1}, []string{"d5b", "d4", "d3", "d1", "feb", "jan"}},
		{"monthly 2 keeps newest of each month plus newest overall", RetentionPolicy{Monthly: 2}, []string{"d5b", "d4", "d3", "d1", "jan"}},
		{"daily and monthly combine", RetentionPolicy{Daily: 1, Monthly: 3}, []string{"d5b", "d4", "d3", "d1"}},
		{"weekly buckets are ISO weeks", RetentionPolicy{Weekly: 1}, []string{"d5b", "d4", "d3", "d1", "feb", "jan"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExpiredByPolicy(items, c.policy)
			if strings.Join(sorted(got), ",") != strings.Join(sorted(c.want), ",") {
				t.Fatalf("expired = %v, want %v", got, c.want)
			}
		})
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestRetentionPolicy_GapDoesNotShrinkKeep(t *testing.T) {
	base := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	items := []RetentionItem{{"a", base}, {"b", base.AddDate(0, 0, -10)}, {"c", base.AddDate(0, 0, -20)}, {"d", base.AddDate(0, 0, -30)}}
	got := ExpiredByPolicy(items, RetentionPolicy{Daily: 3})
	if len(got) != 1 || got[0] != "d" {
		t.Fatalf("expired = %v, want only d (three days that have backups are kept even across a gap)", got)
	}
}

func TestSealer_RoundTrip(t *testing.T) {
	payload := make([]byte, 3<<20)
	_, _ = rand.Read(payload[:1<<20])
	cases := []struct {
		name   string
		sealer *Sealer
		codec  string
	}{
		{"zstd only", &Sealer{}, CodecZstd},
		{"zstd and age", newTestSealer(t), CodecZstdAge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.sealer.Codec() != c.codec {
				t.Fatalf("codec = %q", c.sealer.Codec())
			}
			sealed, err := io.ReadAll(c.sealer.Seal(bytes.NewReader(payload)))
			if err != nil {
				t.Fatal(err)
			}
			if c.codec == CodecZstdAge && bytes.Contains(sealed, payload[:64]) {
				t.Fatal("sealed output contains plaintext")
			}
			r, err := OpenSealed(bytes.NewReader(sealed), c.codec, c.sealer.Identities)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(r)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("round trip mismatch: err=%v len=%d", err, len(got))
			}
		})
	}
}

func TestOpenSealed_FailsLoudly(t *testing.T) {
	s := newTestSealer(t)
	payload := bytes.Repeat([]byte("levelrail backup payload "), 50000)
	sealed, _ := io.ReadAll(s.Seal(bytes.NewReader(payload)))

	read := func(b []byte, codec string, ids []age.Identity) error {
		r, err := OpenSealed(bytes.NewReader(b), codec, ids)
		if err != nil {
			return err
		}
		_, err = io.ReadAll(r)
		return err
	}
	other, _ := age.GenerateX25519Identity()
	flipped := append([]byte(nil), sealed...)
	flipped[len(flipped)/2] ^= 0xff

	cases := []struct {
		name  string
		bytes []byte
		ids   []age.Identity
	}{
		{"truncated", sealed[:len(sealed)-40], s.Identities},
		{"truncated early", sealed[:len(sealed)/3], s.Identities},
		{"bit flipped", flipped, s.Identities},
		{"wrong key", sealed, []age.Identity{other}},
		{"no key", sealed, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := read(c.bytes, CodecZstdAge, c.ids); err == nil {
				t.Fatal("expected an error, read succeeded")
			}
		})
	}
	if err := read(sealed, CodecZstdAge, nil); !errors.Is(err, ErrNoDecryptionKey) {
		t.Fatalf("no-key error = %v, want ErrNoDecryptionKey", err)
	}

	plain, _ := io.ReadAll((&Sealer{}).Seal(bytes.NewReader(payload)))
	if err := read(plain[:len(plain)-8], CodecZstd, nil); err == nil {
		t.Fatal("truncated zstd stream read without error")
	}
}

func TestLoadSealer_GeneratesAndReusesKey(t *testing.T) {
	dir := t.TempDir()
	none := func(string) (string, bool) { return "", false }
	a, err := LoadSealer(none, dir)
	if err != nil || a.Codec() != CodecZstdAge {
		t.Fatalf("first load: %v codec=%q", err, a.Codec())
	}
	b, err := LoadSealer(none, dir)
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := io.ReadAll(a.Seal(strings.NewReader("hello")))
	r, err := OpenSealed(bytes.NewReader(sealed), CodecZstdAge, b.Identities)
	if err != nil {
		t.Fatalf("second load cannot open first load's object: %v", err)
	}
	if got, _ := io.ReadAll(r); string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	off, err := LoadSealer(func(k string) (string, bool) { return "off", k == EnvBackupEncryption }, dir)
	if err != nil || off.Codec() != CodecZstd {
		t.Fatalf("off: %v codec=%q", err, off.Codec())
	}
}

type volTestEnv struct {
	hist    *memHistory
	objs    *memObjects
	vols    *memVolumes
	runner  *Runner
	sealer  *Sealer
	started time.Time
}

func newVolTestEnv(t *testing.T, store Opts) *volTestEnv {
	t.Helper()
	e := &volTestEnv{hist: newMemHistory(), objs: newMemObjects(), vols: newMemVolumes(), sealer: newTestSealer(t)}
	e.started = time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	e.vols.put("app-web-data", "index.html", memFile{data: []byte("<h1>hi</h1>"), mode: 0o640, uid: 1234})
	e.vols.put("app-web-data", "sub/blob.bin", memFile{data: bytes.Repeat([]byte{7}, 100000), mode: 0o600, uid: 0})
	e.vols.put("app-web-data", "link", memFile{mode: 0o777, link: "index.html"})
	var hs HistoryStore = e.hist
	if store.noSeal {
		hs = noSealStore{e.hist}
	}
	e.runner = &Runner{
		Store: hs, Secrets: newTestSecrets(), Uploader: e.objs, VolumeArchiver: e.vols,
		Now:    func() time.Time { return e.started },
		Volume: &VolumeBackupOptions{Sealer: e.sealer, Policies: e.hist, Attempts: store.attempts, MaxBytes: store.maxBytes},
	}
	return e
}

type Opts struct {
	noSeal   bool
	attempts int
	maxBytes int64
}

func (e *volTestEnv) drill() *DrillRunner {
	return &DrillRunner{
		Store: e.hist, Secrets: newTestSecrets(), Downloader: e.objs, Prober: e.objs, Identities: e.sealer.Identities,
		Restorer: e.vols, Archiver: e.vols, Volumes: e.vols, Remover: e.vols,
	}
}

func TestRunVolumeBackup_SealedRoundTripAndManifest(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	if err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", ""); err != nil {
		t.Fatal(err)
	}
	row := e.hist.rows["bkh_1"]
	if row.Status != store.BackupStatusSucceeded || row.Codec != CodecZstdAge {
		t.Fatalf("row = %+v", row)
	}
	if !strings.HasSuffix(row.ObjectKey, ".tar.zst.age") || !strings.Contains(row.ObjectKey, "bkh_1") {
		t.Fatalf("object key %q should be unique per backup and name the codec", row.ObjectKey)
	}
	if row.ManifestFiles != 3 || row.ManifestDigest == "" || row.ManifestBytes != int64(len("<h1>hi</h1>")+100000) {
		t.Fatalf("manifest = %d files %d bytes digest %q", row.ManifestFiles, row.ManifestBytes, row.ManifestDigest)
	}
	if row.SizeBytes != int64(len(e.objs.objects[row.ObjectKey])) {
		t.Fatalf("recorded size %d != stored %d", row.SizeBytes, len(e.objs.objects[row.ObjectKey]))
	}
	if bytes.Contains(e.objs.objects[row.ObjectKey], []byte("<h1>hi</h1>")) {
		t.Fatal("stored object holds plaintext")
	}

	rr := &RestoreRunner{Store: restoreAdapter{e.hist}, Secrets: newTestSecrets(), Downloader: e.objs, VolumeRestorer: e.vols, Identities: e.sealer.Identities}
	if err := downloadAndRestoreVolume(context.Background(), e.hist, rr.Secrets, rr.Downloader, rr.VolumeRestorer, rr.Identities, "app-web-new", "bkh_1"); err != nil {
		t.Fatal(err)
	}
	got := e.vols.vols["app-web-new"]
	if string(got["index.html"].data) != "<h1>hi</h1>" || got["index.html"].uid != 1234 || got["link"].link != "index.html" || got["sub/blob.bin"].mode != 0o600 {
		t.Fatalf("restored volume differs: %+v", got)
	}
}

type restoreAdapter struct{ *memHistory }

func (restoreAdapter) StartRestoreHistory(context.Context, store.RestoreHistory) error { return nil }
func (restoreAdapter) FinishRestoreHistory(context.Context, string, string, string, string) error {
	return nil
}

func TestRunVolumeBackup_StoreCannotRecordCodecFails(t *testing.T) {
	e := newVolTestEnv(t, Opts{noSeal: true})
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	if err == nil || !strings.Contains(err.Error(), "codec") {
		t.Fatalf("err = %v, want a codec recording failure", err)
	}
	if e.hist.rows["bkh_1"].Status != store.BackupStatusFailed {
		t.Fatalf("status = %q, want failed", e.hist.rows["bkh_1"].Status)
	}
}

func TestRunVolumeBackup_HalfSucceededUploadIsRetriedFromFreshArchive(t *testing.T) {
	e := newVolTestEnv(t, Opts{attempts: 3})
	e.objs.failUploads, e.objs.failAfter = 2, 1000
	if err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", ""); err != nil {
		t.Fatal(err)
	}
	if e.objs.uploads != 3 {
		t.Fatalf("uploads = %d, want 3 (two failed mid-object, one complete)", e.objs.uploads)
	}
	row := e.hist.rows["bkh_1"]
	if row.Status != store.BackupStatusSucceeded {
		t.Fatalf("status = %q", row.Status)
	}
	if _, err := e.drill().RunDrill(context.Background(), "bkd_1", "bkh_1", store.DrillTriggerManual); err != nil {
		t.Fatalf("retried backup does not restore: %v", err)
	}
}

func TestRunVolumeBackup_UploadFailsAfterAllAttempts(t *testing.T) {
	e := newVolTestEnv(t, Opts{attempts: 2})
	e.objs.failUploads, e.objs.failAfter = 5, 10
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	if err == nil {
		t.Fatal("expected failure")
	}
	if e.objs.uploads != 2 || e.hist.rows["bkh_1"].Status != store.BackupStatusFailed {
		t.Fatalf("uploads=%d status=%q", e.objs.uploads, e.hist.rows["bkh_1"].Status)
	}
	if len(e.objs.objects) != 0 {
		t.Fatalf("a failed upload left objects behind: %d", len(e.objs.objects))
	}
}

func TestRunVolumeBackup_SizeBudget(t *testing.T) {
	e := newVolTestEnv(t, Opts{maxBytes: 50000, attempts: 3})
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	if !errors.Is(err, ErrBackupTooLarge) && (err == nil || !strings.Contains(err.Error(), "size budget")) {
		t.Fatalf("err = %v, want size budget error", err)
	}
	if e.objs.uploads != 1 {
		t.Fatalf("uploads = %d: an over budget archive must not be retried", e.objs.uploads)
	}
	if len(e.objs.objects) != 0 {
		t.Fatal("over budget archive was stored")
	}
}

func TestRunVolumeBackup_Timeout(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	e.runner.Volume.Timeout = time.Nanosecond
	time.Sleep(time.Millisecond)
	if err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", ""); err == nil {
		t.Fatal("expected the timeout to fail the backup")
	}
}

type recordingQuiescer struct {
	events []string
	failOn string
}

func (q *recordingQuiescer) Begin(_ context.Context, svc string, p store.VolumeBackupPolicy) (func(context.Context) error, error) {
	q.events = append(q.events, "begin:"+svc+":"+p.PreHook+":"+p.Quiesce)
	if q.failOn == "begin" {
		return nil, errors.New("pre-backup hook failed")
	}
	return func(context.Context) error { q.events = append(q.events, "end"); return nil }, nil
}

func TestRunVolumeBackup_QuiesceHooks(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	q := &recordingQuiescer{}
	e.runner.Volume.Quiesce = q
	e.hist.policies["web/data"] = store.VolumeBackupPolicy{PreHook: "sync", Quiesce: store.VolumeQuiescePause}
	if err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Join(q.events, ",") != "begin:web:sync:pause,end" {
		t.Fatalf("events = %v", q.events)
	}

	q2 := &recordingQuiescer{failOn: "begin"}
	e.runner.Volume.Quiesce = q2
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_2", "web", "data", "app-web-data", "bkt_test", "")
	if err == nil || !strings.Contains(err.Error(), "pre-backup hook") {
		t.Fatalf("err = %v", err)
	}
	if e.hist.rows["bkh_2"].Status != store.BackupStatusFailed {
		t.Fatal("a failing pre-hook must fail the backup rather than snapshot an inconsistent volume")
	}
}

func TestDrill_VolumePassesAndCleansUp(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	_ = e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	d, err := e.drill().RunDrill(context.Background(), "bkd_1", "bkh_1", store.DrillTriggerScheduled)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != store.DrillStatusPassed || !d.ObjectOK || !d.ChecksumOK || !d.RestoreOK || !d.ContentOK || d.Files != 3 {
		t.Fatalf("drill = %+v", d)
	}
	if e.vols.exists("drill-bkd_1") {
		t.Fatal("scratch volume was not destroyed")
	}
	if got := e.hist.drills["bkd_1"]; got.Status != store.DrillStatusPassed || got.Trigger != store.DrillTriggerScheduled {
		t.Fatalf("stored drill = %+v", got)
	}
}

func TestDrill_CorruptionFailsLoudly(t *testing.T) {
	cases := []struct {
		name    string
		damage  func(e *volTestEnv, key string)
		stage   string
		wantErr string
	}{
		{"truncated object", func(e *volTestEnv, k string) { e.objs.truncate(k, len(e.objs.objects[k])-100) }, DrillStageObject, "truncated"},
		{"missing object", func(e *volTestEnv, k string) { _ = e.objs.Delete(context.Background(), Destination{}, k) }, DrillStageObject, "missing"},
		{"flipped byte same size", func(e *volTestEnv, k string) { e.objs.flip(k, len(e.objs.objects[k])/2) }, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newVolTestEnv(t, Opts{})
			_ = e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
			c.damage(e, e.hist.rows["bkh_1"].ObjectKey)

			var failed []store.BackupDrill
			dr := e.drill()
			dr.OnFailure = func(_ context.Context, d store.BackupDrill) { failed = append(failed, d) }
			d, err := dr.RunDrill(context.Background(), "bkd_1", "bkh_1", store.DrillTriggerManual)
			if err == nil || d.Status != store.DrillStatusFailed {
				t.Fatalf("expected a failed drill, got status %q err %v", d.Status, err)
			}
			if c.stage != "" && d.Stage != c.stage {
				t.Fatalf("stage = %q, want %q", d.Stage, c.stage)
			}
			if c.wantErr != "" && !strings.Contains(d.Error, c.wantErr) {
				t.Fatalf("error %q should mention %q", d.Error, c.wantErr)
			}
			if len(failed) != 1 {
				t.Fatalf("OnFailure calls = %d, want 1", len(failed))
			}
			if e.vols.exists("drill-bkd_1") {
				t.Fatal("scratch volume leaked after a failed drill")
			}
		})
	}
}

func TestDrill_LegacyRawBackupWithoutManifest(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	e.runner.Volume = nil
	if err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", ""); err != nil {
		t.Fatal(err)
	}
	if e.hist.rows["bkh_1"].Codec != "" {
		t.Fatal("legacy backup must stay raw")
	}
	d, err := e.drill().RunDrill(context.Background(), "bkd_1", "bkh_1", store.DrillTriggerManual)
	if err != nil || !d.ContentOK {
		t.Fatalf("legacy drill: %v %+v", err, d)
	}
}

func TestDrill_RefusesFailedBackup(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	e.hist.rows["bkh_x"] = store.BackupHistory{ID: "bkh_x", Status: store.BackupStatusFailed, ResourceKind: store.BackupResourceKindVolume, TargetID: "bkt_test"}
	if _, err := e.drill().RunDrill(context.Background(), "bkd_1", "bkh_x", store.DrillTriggerManual); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestDrillScheduler_DrillsDueResourcesOnce(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	_ = e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	s := &DrillScheduler{Store: e.hist, Secrets: newTestSecrets(), Runner: e.drill(), Prober: e.objs, Downloader: e.objs, Interval: 24 * time.Hour, Now: func() time.Time { return now }}
	s.Runner.Now = s.Now
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.hist.drills) != 1 {
		t.Fatalf("drills = %d, want 1", len(e.hist.drills))
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.hist.drills) != 1 {
		t.Fatalf("second tick inside the interval drilled again: %d", len(e.hist.drills))
	}
	now = now.Add(25 * time.Hour)
	_ = s.Tick(context.Background())
	if len(e.hist.drills) != 2 {
		t.Fatalf("drills after interval = %d, want 2", len(e.hist.drills))
	}
}

func TestDrillScheduler_SweepsInterruptedBackups(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	_ = e.runner.RunVolumeBackup(context.Background(), "bkh_done", "web", "data", "app-web-data", "bkt_test", "")
	whole := e.hist.rows["bkh_done"]

	old := e.started.Format(time.RFC3339)
	e.hist.rows["bkh_whole"] = store.BackupHistory{ID: "bkh_whole", Status: store.BackupStatusRunning, TargetID: "bkt_test", ObjectKey: whole.ObjectKey, StartedAt: old}
	e.hist.rows["bkh_gone"] = store.BackupHistory{ID: "bkh_gone", Status: store.BackupStatusRunning, TargetID: "bkt_test", ObjectKey: "volumes/none", StartedAt: old}
	e.hist.rows["bkh_fresh"] = store.BackupHistory{ID: "bkh_fresh", Status: store.BackupStatusRunning, TargetID: "bkt_test", ObjectKey: "x", StartedAt: e.started.Add(10 * time.Hour).Format(time.RFC3339)}

	s := &DrillScheduler{Store: e.hist, Secrets: newTestSecrets(), Prober: e.objs, Downloader: e.objs, StaleAfter: time.Hour, Now: func() time.Time { return e.started.Add(10*time.Hour + time.Minute) }}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r := e.hist.rows["bkh_whole"]; r.Status != store.BackupStatusSucceeded || r.ChecksumSHA256 != whole.ChecksumSHA256 || r.SizeBytes != whole.SizeBytes {
		t.Fatalf("whole object not finalized from the bucket: %+v", r)
	}
	if r := e.hist.rows["bkh_gone"]; r.Status != store.BackupStatusFailed || !strings.Contains(r.Error, "interrupted") {
		t.Fatalf("missing object not failed: %+v", r)
	}
	if r := e.hist.rows["bkh_fresh"]; r.Status != store.BackupStatusRunning {
		t.Fatalf("a recent running backup was touched: %+v", r)
	}
}

func TestScheduler_PruneGFS(t *testing.T) {
	e := newVolTestEnv(t, Opts{})
	e.hist.policies["web/data"] = store.VolumeBackupPolicy{RetainDaily: 2}
	base := time.Date(2026, 10, 10, 2, 0, 0, 0, time.UTC)
	for i, id := range []string{"b0", "b1", "b2", "b3"} {
		k := "volumes/web/data/" + id
		e.objs.objects[k] = []byte(id)
		e.hist.rows[id] = store.BackupHistory{ID: id, Status: store.BackupStatusSucceeded, ServiceName: "web", VolumeName: "data", TargetID: "bkt_test", ObjectKey: k, StartedAt: base.AddDate(0, 0, -i).Format(time.RFC3339)}
	}
	s := &Scheduler{Runner: e.runner, Deleter: e.objs, Policies: e.hist, GFS: e.hist, Logger: nil}
	s.pruneGFS(context.Background(), store.ServiceVolumeBackupConfig{ServiceName: "web", VolumeName: "data"})
	if _, ok := e.hist.rows["b0"]; !ok {
		t.Fatal("newest removed")
	}
	if _, ok := e.hist.rows["b1"]; !ok {
		t.Fatal("second newest removed")
	}
	for _, id := range []string{"b2", "b3"} {
		if _, ok := e.hist.rows[id]; ok {
			t.Fatalf("%s should be pruned", id)
		}
		if _, ok := e.objs.objects["volumes/web/data/"+id]; ok {
			t.Fatalf("%s object should be deleted", id)
		}
	}
}
