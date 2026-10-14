package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func measureOutput(content, path string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%d\n%s  %s\n", len(content), hex.EncodeToString(sum[:]), path)
}

func isMeasure(cmd []string) bool {
	return len(cmd) == 4 && cmd[0] == "sh" && strings.Contains(cmd[2], "sha256sum")
}

// meiliEngine plays a Meilisearch container whose dump task finishes after
// pendingPolls polls; catContent overrides what cat streams.
func meiliEngine(content string, pendingPolls int, taskStatus, catContent string) func([]string) (string, error) {
	var polls int32
	const path = "/meili_data/dumps/20261014-120000000.dump"
	return func(cmd []string) (string, error) {
		if m, p, ok := httpRequestOf(cmd); ok {
			switch {
			case m == "GET" && p == "/stats":
				return httpOK(`{"indexes":{"movies":{"numberOfDocuments":3},"books":{"numberOfDocuments":2}}}`), nil
			case m == "POST" && p == "/dumps":
				return httpStatus(202, `{"taskUid":7,"status":"enqueued"}`), nil
			case m == "GET" && p == "/tasks/7":
				if int(atomic.AddInt32(&polls, 1)) <= pendingPolls {
					return httpOK(`{"uid":7,"status":"processing"}`), nil
				}
				return httpOK(`{"uid":7,"status":"` + taskStatus + `","details":{"dumpUid":"20261014-120000000"},"error":{"message":"disk full"}}`), nil
			}
			return httpStatus(404, `{}`), nil
		}
		switch {
		case isMeasure(cmd):
			return measureOutput(content, path), nil
		case len(cmd) == 2 && cmd[0] == "cat" && cmd[1] == path:
			return catContent, nil
		case len(cmd) == 3 && cmd[0] == "rm":
			return "", nil
		}
		return "", fmt.Errorf("unexpected command %q", cmd)
	}
}

func TestDumpSearch_Meilisearch_StreamsVerifiedDumpAndCleansUp(t *testing.T) {
	content := "\x1f\x8bdump-bytes"
	rt := newScriptedRuntime(meiliEngine(content, 2, "succeeded", content))
	d := &ContainerDumper{Runtime: rt, SearchPollInterval: time.Millisecond}

	rc, err := d.Dump(context.Background(), store.EngineMeilisearch, "db-search")
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	if string(got) != content {
		t.Fatalf("dump = %q, want %q", got, content)
	}
	if cs, ok := rc.(contentSummarizer); !ok || cs.ContentSummary() != `{"books":2,"movies":3}` {
		t.Fatalf("content summary = %v", rc)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	log := strings.Join(rt.commandLog(), "\n")
	for _, want := range []string{"POST /dumps", "GET /tasks/7", "cat /meili_data/dumps/20261014-120000000.dump", "rm -f /meili_data/dumps/20261014-120000000.dump"} {
		if !strings.Contains(log, want) {
			t.Errorf("command log missing %q:\n%s", want, log)
		}
	}
}

func TestDumpSearch_Meilisearch_HalfSucceeded(t *testing.T) {
	content := "\x1f\x8bdump-bytes-that-are-long"
	tests := []struct {
		name     string
		polls    int
		status   string
		cat      string
		wantDump string
		wantRead string
	}{
		{name: "task never finishes", polls: 1 << 20, status: "succeeded", cat: content, wantDump: "timed out"},
		{name: "task failed", polls: 0, status: "failed", cat: content, wantDump: "disk full"},
		{name: "task canceled", polls: 0, status: "canceled", cat: content, wantDump: "canceled"},
		{name: "file truncated while streaming", polls: 0, status: "succeeded", cat: content[:10], wantRead: "truncated"},
		{name: "file altered while streaming", polls: 0, status: "succeeded", cat: strings.Replace(content, "dump", "DUMP", 1), wantRead: "checksum mismatch"},
		{name: "file grew while streaming", polls: 0, status: "succeeded", cat: content + "x", wantRead: "grew"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := newScriptedRuntime(meiliEngine(content, tc.polls, tc.status, tc.cat))
			d := &ContainerDumper{Runtime: rt, SearchTaskTimeout: 20 * time.Millisecond, SearchPollInterval: time.Millisecond}
			rc, err := d.Dump(context.Background(), store.EngineMeilisearch, "db-search")
			if tc.wantDump != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantDump) {
					t.Fatalf("Dump() error = %v, want it to mention %q", err, tc.wantDump)
				}
				if strings.Contains(strings.Join(rt.commandLog(), "\n"), "cat ") {
					t.Fatal("a failed snapshot task must never stream a file")
				}
				return
			}
			if err != nil {
				t.Fatalf("Dump() error = %v", err)
			}
			_, readErr := io.ReadAll(rc)
			_ = rc.Close()
			if readErr == nil || !strings.Contains(readErr.Error(), tc.wantRead) {
				t.Fatalf("read error = %v, want it to mention %q", readErr, tc.wantRead)
			}
		})
	}
}

// strictUploader fails like a real multipart upload when its reader errors.
type strictUploader struct{ body bytes.Buffer }

func (u *strictUploader) Upload(_ context.Context, _ Destination, _ string, r io.Reader, _ int64) error {
	_, err := io.Copy(&u.body, r)
	return err
}

type summaryHistoryStore struct {
	*fakeHistoryStore
	summaries map[string]string
}

func (s *summaryHistoryStore) SetBackupContentSummary(_ context.Context, id, summary string) error {
	s.summaries[id] = summary
	return nil
}

func TestRunBackup_Search_TruncatedDumpIsNeverMarkedComplete(t *testing.T) {
	content := "\x1f\x8bdump-bytes-that-are-long"
	hs := &summaryHistoryStore{fakeHistoryStore: &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}, summaries: map[string]string{}}
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &ContainerDumper{Runtime: newScriptedRuntime(meiliEngine(content, 0, "succeeded", content[:7])), SearchPollInterval: time.Millisecond},
		Uploader: &strictUploader{},
	}
	err := r.RunBackup(context.Background(), "bkh_1", "search", store.EngineMeilisearch, "db-search", "bkt_test")
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("RunBackup() error = %v, want a truncation failure", err)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hs.finished)
	}
	if len(hs.summaries) != 0 {
		t.Fatalf("a failed backup must not record content counts, got %v", hs.summaries)
	}
}

func TestRunBackup_Search_RecordsStableSummary(t *testing.T) {
	content := "\x1f\x8bdump-bytes"
	hs := &summaryHistoryStore{fakeHistoryStore: &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}, summaries: map[string]string{}}
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &ContainerDumper{Runtime: newScriptedRuntime(meiliEngine(content, 0, "succeeded", content)), SearchPollInterval: time.Millisecond},
		Uploader: &strictUploader{},
	}
	if err := r.RunBackup(context.Background(), "bkh_1", "search", store.EngineMeilisearch, "db-search", "bkt_test"); err != nil {
		t.Fatalf("RunBackup() error = %v", err)
	}
	if hs.summaries["bkh_1"] != `{"books":2,"movies":3}` {
		t.Fatalf("summary = %q", hs.summaries["bkh_1"])
	}
}

func qdrantEngine(content, name string, changing bool) func([]string) (string, error) {
	const dir = "/qdrant/storage/snapshots"
	var countCalls int32
	return func(cmd []string) (string, error) {
		if m, p, ok := httpRequestOf(cmd); ok {
			switch {
			case m == "GET" && p == "/collections":
				return httpOK(`{"result":{"collections":[{"name":"docs"}]},"status":"ok"}`), nil
			case m == "POST" && p == "/collections/docs/points/count":
				n := 10
				if changing && atomic.AddInt32(&countCalls, 1) > 1 {
					n = 11
				}
				return httpOK(fmt.Sprintf(`{"result":{"count":%d}}`, n)), nil
			case m == "POST" && p == "/snapshots?wait=true":
				return httpOK(`{"result":{"name":"` + name + `","size":10},"status":"ok"}`), nil
			case m == "DELETE":
				return httpOK(`{"result":true}`), nil
			}
			return httpStatus(404, `{}`), nil
		}
		switch {
		case cmd[0] == "find":
			return dir + "/" + name + "\n", nil
		case isMeasure(cmd):
			return measureOutput(content, cmd[3]), nil
		case cmd[0] == "cat":
			return content, nil
		}
		return "", fmt.Errorf("unexpected command %q", cmd)
	}
}

func TestDumpSearch_Qdrant(t *testing.T) {
	content := "tar-bytes"
	rt := newScriptedRuntime(qdrantEngine(content, "full-snapshot-2026-10-14.snapshot", false))
	rc, err := (&ContainerDumper{Runtime: rt}).Dump(context.Background(), store.EngineQdrant, "db-vec")
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	if got, err := io.ReadAll(rc); err != nil || string(got) != content {
		t.Fatalf("dump = %q, %v", got, err)
	}
	if rc.(contentSummarizer).ContentSummary() != `{"docs":10}` {
		t.Fatalf("summary = %q", rc.(contentSummarizer).ContentSummary())
	}
	_ = rc.Close()
	log := strings.Join(rt.commandLog(), "\n")
	if !strings.Contains(log, "DELETE /snapshots/full-snapshot-2026-10-14.snapshot") {
		t.Fatalf("snapshot not deleted after streaming:\n%s", log)
	}
}

func TestDumpSearch_Qdrant_ChangingCountsRecordNoSummary(t *testing.T) {
	rt := newScriptedRuntime(qdrantEngine("tar-bytes", "full.snapshot", true))
	rc, err := (&ContainerDumper{Runtime: rt}).Dump(context.Background(), store.EngineQdrant, "db-vec")
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	defer func() { _ = rc.Close() }()
	if s := rc.(contentSummarizer).ContentSummary(); s != "" {
		t.Fatalf("counts moved during the snapshot, summary must be empty, got %q", s)
	}
}

func TestDumpSearch_Qdrant_RefusesUnsafeSnapshotName(t *testing.T) {
	rt := newScriptedRuntime(qdrantEngine("x", "../../etc/passwd", false))
	if _, err := (&ContainerDumper{Runtime: rt}).Dump(context.Background(), store.EngineQdrant, "db-vec"); err == nil {
		t.Fatal("an unsafe snapshot name must fail the dump")
	}
}

func typesenseEngine(content string, success bool) func([]string) (string, error) {
	return func(cmd []string) (string, error) {
		if m, p, ok := httpRequestOf(cmd); ok {
			switch {
			case m == "GET" && p == "/collections":
				return httpOK(`[{"name":"products","num_documents":4}]`), nil
			case m == "POST" && p == "/operations/snapshot?snapshot_path=%2Fdata%2F.backup-snapshot":
				return httpOK(fmt.Sprintf(`{"success":%t}`, success)), nil
			}
			return httpStatus(404, `{}`), nil
		}
		switch {
		case cmd[0] == "rm":
			return "", nil
		case cmd[0] == "sh" && strings.Contains(cmd[2], "tar -C"):
			return "", nil
		case isMeasure(cmd):
			return measureOutput(content, cmd[3]), nil
		case cmd[0] == "cat" && cmd[1] == "/data/.backup-snapshot.tar":
			return content, nil
		}
		return "", fmt.Errorf("unexpected command %q", cmd)
	}
}

func TestDumpSearch_Typesense(t *testing.T) {
	rt := newScriptedRuntime(typesenseEngine("tar-bytes", true))
	rc, err := (&ContainerDumper{Runtime: rt}).Dump(context.Background(), store.EngineTypesense, "db-ts")
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	if got, err := io.ReadAll(rc); err != nil || string(got) != "tar-bytes" {
		t.Fatalf("dump = %q, %v", got, err)
	}
	_ = rc.Close()
	log := rt.commandLog()
	if last := log[len(log)-1]; last != "rm -rf /data/.backup-snapshot /data/.backup-snapshot.tar" {
		t.Fatalf("last command = %q, want snapshot cleanup", last)
	}
}

func TestDumpSearch_Typesense_UnsuccessfulSnapshotFails(t *testing.T) {
	rt := newScriptedRuntime(typesenseEngine("tar-bytes", false))
	_, err := (&ContainerDumper{Runtime: rt}).Dump(context.Background(), store.EngineTypesense, "db-ts")
	if err == nil || !strings.Contains(err.Error(), "unsuccessful") {
		t.Fatalf("Dump() error = %v", err)
	}
}

func TestDumpSearch_EngineAPIErrorFails(t *testing.T) {
	rt := newScriptedRuntime(func([]string) (string, error) {
		return httpStatus(401, `{"message":"The provided API key is invalid."}`), nil
	})
	_, err := (&ContainerDumper{Runtime: rt}).Dump(context.Background(), store.EngineMeilisearch, "db-search")
	var apiErr *searchAPIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("Dump() error = %v, want the 401 surfaced", err)
	}
}
