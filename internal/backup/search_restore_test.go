package backup

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

type restorePlay struct {
	uploadErr  error
	boots      string
	healthyNow func(rt *scriptedRuntime) bool
}

func restoreEngine(rt **scriptedRuntime, play restorePlay) func([]string) (string, error) {
	return func(cmd []string) (string, error) {
		if _, p, ok := httpRequestOf(cmd); ok {
			if p == searchHealthPath[store.EngineMeilisearch] || p == searchHealthPath[store.EngineQdrant] {
				if play.healthyNow != nil && !play.healthyNow(*rt) {
					return httpStatus(503, `{}`), nil
				}
				return httpOK(`{"status":"available"}`), nil
			}
			return httpStatus(404, `{}`), nil
		}
		switch {
		case cmd[0] == "sh" && strings.Contains(cmd[2], `cat > "$U"`):
			return "", play.uploadErr
		case cmd[0] == "cat" && strings.HasSuffix(cmd[1], database.RestoreBootsName):
			return play.boots, nil
		case cmd[0] == "sh":
			return "", nil
		}
		return "", nil
	}
}

func newRestoreRuntime(play restorePlay) *scriptedRuntime {
	var rt *scriptedRuntime
	rt = newScriptedRuntime(restoreEngine(&rt, play))
	return rt
}

func hasCommand(log []string, substr string) bool {
	for _, l := range log {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func TestRestoreSearch_LoadsConfirmsAndKeepsNoRollback(t *testing.T) {
	rt := newRestoreRuntime(restorePlay{boots: "boot\n"})
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Second, ReadyInterval: time.Millisecond}
	if err := r.Restore(context.Background(), store.EngineMeilisearch, "db-search", strings.NewReader("dump-bytes")); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if len(rt.uploads) != 1 || rt.uploads[0] != "dump-bytes" {
		t.Fatalf("uploads = %q", rt.uploads)
	}
	if rt.stops != 1 || rt.starts != 1 {
		t.Fatalf("stops/starts = %d/%d, want one restart", rt.stops, rt.starts)
	}
	log := rt.commandLog()
	if !hasCommand(log, ".restore-importing") || hasCommand(log, database.RestoreRollbackName) {
		t.Fatalf("want a confirm and no rollback, log:\n%s", strings.Join(log, "\n"))
	}
}

func TestRestoreSearch_HalfSucceeded(t *testing.T) {
	tests := []struct {
		name         string
		play         func() restorePlay
		wantErr      string
		wantRollback bool
		wantRestarts int
	}{
		{
			name:         "invalid or truncated artifact is never staged",
			play:         func() restorePlay { return restorePlay{uploadErr: errExit1} },
			wantErr:      "upload or validate artifact",
			wantRestarts: 0,
		},
		{
			name:         "engine crashed while loading and came back",
			play:         func() restorePlay { return restorePlay{boots: "boot\nboot\n"} },
			wantErr:      "put back",
			wantRollback: true,
			wantRestarts: 2,
		},
		{
			name:         "engine started without the import",
			play:         func() restorePlay { return restorePlay{boots: ""} },
			wantErr:      "without loading",
			wantRollback: true,
			wantRestarts: 2,
		},
		{
			name: "engine never healthy after the restart",
			play: func() restorePlay {
				return restorePlay{boots: "boot\n", healthyNow: func(rt *scriptedRuntime) bool { return rt.starts == 0 || rt.starts >= 2 }}
			},
			wantErr:      "not ready",
			wantRollback: true,
			wantRestarts: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := newRestoreRuntime(tc.play())
			r := &ContainerRestorer{Runtime: rt, ReadyTimeout: 30 * time.Millisecond, ReadyInterval: time.Millisecond}
			err := r.Restore(context.Background(), store.EngineQdrant, "db-vec", strings.NewReader("snap"))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Restore() error = %v, want it to mention %q", err, tc.wantErr)
			}
			log := rt.commandLog()
			if got := hasCommand(log, database.RestoreRollbackName); got != tc.wantRollback {
				t.Fatalf("rollback requested = %v, want %v; log:\n%s", got, tc.wantRollback, strings.Join(log, "\n"))
			}
			if hasCommand(log, "rm -f '/qdrant/storage/.restore-importing") {
				t.Fatal("a failed restore must never be confirmed")
			}
			if rt.starts != tc.wantRestarts {
				t.Fatalf("starts = %d, want %d", rt.starts, tc.wantRestarts)
			}
		})
	}
}

func TestSearchUploadScript_ValidatesBeforeMarkingPending(t *testing.T) {
	tests := []struct {
		engine, validate, pending string
	}{
		{store.EngineMeilisearch, `tar -tzf "$U"`, "/meili_data/.restore-pending.dump"},
		{store.EngineQdrant, `tar -tf "$U"`, "/qdrant/storage/.restore-pending.snapshot"},
		{store.EngineTypesense, `tar -xf "$U" -C "$S"`, "/data/.restore-pending"},
	}
	for _, tc := range tests {
		t.Run(tc.engine, func(t *testing.T) {
			l, _ := database.SearchEngineLayout(tc.engine)
			cmd := searchUploadScript(tc.engine, l)
			script := cmd[2]
			if !strings.Contains(script, tc.validate) || cmd[4] != tc.pending {
				t.Fatalf("script = %q args = %q", script, cmd[3:])
			}
			if strings.Index(script, tc.validate) > strings.LastIndex(script, `"$P"`) {
				t.Fatal("the artifact must be validated before it is marked pending")
			}
		})
	}
}

func TestSearchHTTPCmd_NeverCarriesTheKey(t *testing.T) {
	tests := []struct {
		engine, shell, keyRef, header string
	}{
		{store.EngineMeilisearch, "sh", `"$MEILI_MASTER_KEY"`, "Authorization: Bearer"},
		{store.EngineQdrant, "bash", `"$QDRANT__SERVICE__API_KEY"`, "api-key:"},
		{store.EngineTypesense, "bash", `"$TYPESENSE_API_KEY"`, "X-TYPESENSE-API-KEY:"},
	}
	for _, tc := range tests {
		t.Run(tc.engine, func(t *testing.T) {
			cmd, err := searchHTTPCmd(tc.engine, "GET", "/stats", "")
			if err != nil {
				t.Fatal(err)
			}
			if cmd[0] != tc.shell || !strings.Contains(cmd[2], tc.keyRef) || !strings.Contains(cmd[2], tc.header) {
				t.Fatalf("cmd = %q", cmd)
			}
			for _, arg := range cmd[3:] {
				if strings.Contains(arg, "KEY") {
					t.Fatalf("argument %q looks like it carries the key", arg)
				}
			}
		})
	}
	if _, err := searchHTTPCmd(store.EnginePostgres, "GET", "/", ""); err == nil {
		t.Fatal("postgres has no HTTP API")
	}
}

func TestParseRawHTTP(t *testing.T) {
	status, body, err := parseRawHTTP([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"))
	if err != nil || status != 200 || string(body) != "hello" {
		t.Fatalf("chunked = %d %q %v", status, body, err)
	}
	status, body, err = parseRawHTTP([]byte("HTTP/1.0 202 Accepted\r\n\r\n{\"taskUid\":1}"))
	if err != nil || status != 202 || string(body) != `{"taskUid":1}` {
		t.Fatalf("http/1.0 = %d %q %v", status, body, err)
	}
	if _, _, err := parseRawHTTP(nil); err == nil {
		t.Fatal("an empty answer must be an error")
	}
}

func TestValidateFormat_SearchEngines(t *testing.T) {
	zeros := make([]byte, 1024)
	tail := append([]byte("x"), zeros...)
	tests := []struct {
		name   string
		engine string
		header []byte
		tail   []byte
		total  int64
		ok     bool
	}{
		{"meili gzip", store.EngineMeilisearch, []byte{0x1f, 0x8b, 8}, nil, 10, true},
		{"meili not gzip", store.EngineMeilisearch, []byte("PK"), nil, 10, false},
		{"qdrant complete tar", store.EngineQdrant, nil, tail, 2048, true},
		{"qdrant truncated tar", store.EngineQdrant, nil, []byte("data"), 2048, false},
		{"typesense odd size", store.EngineTypesense, nil, tail, 2049, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if ok, why := validateFormat(tc.engine, tc.header, tc.tail, tc.total); ok != tc.ok {
				t.Fatalf("validateFormat() = %v (%s), want %v", ok, why, tc.ok)
			}
		})
	}
}

type summaryDrillStore struct {
	DrillStore
	summary string
}

func (s summaryDrillStore) GetBackupContentSummary(context.Context, string) (string, error) {
	return s.summary, nil
}

func TestDrillCheckSearchContent(t *testing.T) {
	var calls int32
	rt := newScriptedRuntime(func([]string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return httpOK(`[{"name":"products","num_documents":4}]`), nil
	})
	tests := []struct {
		summary string
		wantErr bool
	}{
		{"", false},
		{`{"products":4}`, false},
		{`{"products":5}`, true},
		{`{"products":4,"orders":1}`, true},
	}
	for _, tc := range tests {
		r := &DrillRunner{Runtime: rt, Store: summaryDrillStore{summary: tc.summary}}
		n, err := r.checkSearchContent(context.Background(), "drill-1", store.EngineTypesense, "bkh_1")
		if (err != nil) != tc.wantErr {
			t.Fatalf("summary %q: err = %v, wantErr %v", tc.summary, err, tc.wantErr)
		}
		if err == nil && n != 1 {
			t.Fatalf("objects = %d, want 1", n)
		}
	}
}
