package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPositionReader_FindsCoordinatesInBothDumpDialects(t *testing.T) {
	tests := []struct {
		name string
		head string
		want string
	}{
		{"mysql 8.4", "-- Server version\n-- CHANGE REPLICATION SOURCE TO SOURCE_LOG_FILE='mysql-bin.000002', SOURCE_LOG_POS=157;\nCREATE TABLE t(a int);", "mysql-bin.000002:157"},
		{"mariadb", "-- CHANGE MASTER TO MASTER_LOG_FILE='mysql-bin.000010', MASTER_LOG_POS=330;\n", "mysql-bin.000010:330"},
		{"no coordinates", "CREATE TABLE t(a int);", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &positionReader{ReadCloser: io.NopCloser(strings.NewReader(tc.head))}
			if _, err := io.Copy(io.Discard, p); err != nil {
				t.Fatalf("copy: %v", err)
			}
			if got := p.Position(); got != tc.want {
				t.Errorf("Position() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPositionReader_FindsCoordinatesSplitAcrossReads(t *testing.T) {
	line := "-- CHANGE MASTER TO MASTER_LOG_FILE='mysql-bin.000003', MASTER_LOG_POS=99;"
	p := &positionReader{ReadCloser: io.NopCloser(&oneByteReader{s: line})}
	if _, err := io.Copy(io.Discard, p); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got := p.Position(); got != "mysql-bin.000003:99" {
		t.Errorf("Position() = %q", got)
	}
}

type oneByteReader struct {
	s string
	i int
}

func (r *oneByteReader) Read(b []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	b[0] = r.s[r.i]
	r.i++
	return 1, nil
}

func TestIsBinlogCoordinate(t *testing.T) {
	for in, want := range map[string]bool{
		"mysql-bin.000001:157":      true,
		"0/3000060":                 false,
		"":                          false,
		"mysql-bin.1:5":             false,
		"mysql-bin.000001":          false,
		"other-bin.000001:157":      false,
		"mysql-bin.000001:157 junk": false,
	} {
		if got := IsBinlogCoordinate(in); got != want {
			t.Errorf("IsBinlogCoordinate(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBinlogSQLCmd_PassesVariablePartsAsArgs(t *testing.T) {
	cmd := binlogSQLCmd([]string{"mysql-bin.000002", "mysql-bin.000003"}, 157,
		time.Date(2026, 10, 22, 19, 1, 1, 0, time.FixedZone("x", 3600)), "src-db", "dst-db")
	want := []string{"bash", "-c", binlogSQLScript, "bash", "src-db", "dst-db", "157", "2026-10-22 18:01:01", "mysql-bin.000002", "mysql-bin.000003"}
	if strings.Join(cmd, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("cmd = %q, want %q", cmd, want)
	}
	for _, frag := range []string{`*mariadb-binlog) F=$SRC`, `*) F=$DST`, `--rewrite-db="$SRC->$DST"`} {
		if !strings.Contains(binlogSQLScript, frag) {
			t.Errorf("script missing %q", frag)
		}
	}
}

func TestBinlogSQLCmd_HostileNamesStayOutOfScript(t *testing.T) {
	hostile := "x$(touch /tmp/pwned)`id`\";touch /tmp/pwned2;\""
	cmd := binlogSQLCmd([]string{"mysql-bin.000001"}, 4, time.Unix(0, 0), "src", hostile)
	if cmd[2] != binlogSQLScript || strings.Contains(cmd[2], "pwned") {
		t.Fatalf("hostile name leaked into the script text: %q", cmd[2])
	}
	if cmd[5] != hostile {
		t.Fatalf("hostile name should be a plain arg, got %q", cmd[5])
	}
}

type fakeMySQLRuntime struct {
	out      map[string]string
	execs    []string
	flushes  int
	activeSz string
}

func (r *fakeMySQLRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	joined := strings.Join(cmd, " ")
	r.execs = append(r.execs, joined)
	switch {
	case strings.Contains(joined, "UNIX_TIMESTAMP"):
		return io.NopCloser(strings.NewReader("mysql: [Warning] password\n1792695600\n")), nil
	case strings.Contains(joined, "FLUSH BINARY LOGS"):
		r.flushes++
		return io.NopCloser(strings.NewReader("")), nil
	case strings.Contains(joined, `echo "$f $(wc -c`):
		return io.NopCloser(strings.NewReader("mysql-bin.000001 100\nmysql-bin.000002 200\nnoise 1\n")), nil
	case strings.Contains(joined, "wc -c <") && strings.Contains(joined, "tail -n 1"):
		return io.NopCloser(strings.NewReader(r.activeSz)), nil
	case cmd[0] == "cat":
		return io.NopCloser(strings.NewReader("data-of-" + cmd[1][strings.LastIndex(cmd[1], "/")+1:])), nil
	}
	return nil, errors.New("unexpected exec " + joined)
}

func (r *fakeMySQLRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}

func TestMySQLWindowEnd_ReadsClockBeforeRotating(t *testing.T) {
	rt := &fakeMySQLRuntime{}
	end, err := MySQLWindowEnd(context.Background(), rt, "db-x")
	if err != nil {
		t.Fatalf("MySQLWindowEnd() error = %v", err)
	}
	if end.Unix() != 1792695600 || end.Location() != time.UTC {
		t.Errorf("end = %v", end)
	}
	script := rt.execs[0]
	if strings.Index(script, "UNIX_TIMESTAMP") > strings.Index(script, "FLUSH BINARY LOGS") {
		t.Errorf("the clock must be read before the rotation so every event up to it is in a closed file:\n%s", script)
	}
}

func TestBinlogShipper_ShipsOnlyMissingClosedFilesAndSkipsIdleRotation(t *testing.T) {
	rt := &fakeMySQLRuntime{activeSz: "158"}
	up := &recordingUploader{}
	s := &BinlogShipper{
		Runtime:  rt,
		Uploader: up,
		Lister:   staticLister{objs: []RemoteObject{{Key: "db1/binlog/mysql-bin.000001", Size: 100}}},
	}
	n, err := s.Ship(context.Background(), Destination{}, "db1", "db-db1")
	if err != nil {
		t.Fatalf("Ship() error = %v", err)
	}
	if n != 1 || len(up.keys) != 1 || up.keys[0] != "db1/binlog/mysql-bin.000002" {
		t.Fatalf("shipped %d %v, want only mysql-bin.000002", n, up.keys)
	}
	if rt.flushes != 1 {
		t.Fatalf("first ship flushes = %d, want 1", rt.flushes)
	}
	if _, err := s.Ship(context.Background(), Destination{}, "db1", "db-db1"); err != nil {
		t.Fatalf("second Ship() error = %v", err)
	}
	if rt.flushes != 1 {
		t.Errorf("an unchanged active binlog was rotated again: flushes = %d", rt.flushes)
	}
	rt.activeSz = "400"
	if _, err := s.Ship(context.Background(), Destination{}, "db1", "db-db1"); err != nil {
		t.Fatalf("third Ship() error = %v", err)
	}
	if rt.flushes != 2 {
		t.Errorf("a grown active binlog was not rotated: flushes = %d", rt.flushes)
	}
}

type recordingDeleter struct{ keys []string }

func (d *recordingDeleter) Delete(_ context.Context, _ Destination, key string) error {
	d.keys = append(d.keys, key)
	return nil
}

func TestPruneRemoteBinlogs_KeepsFileTheBaseBackupStartsIn(t *testing.T) {
	lister := staticLister{objs: []RemoteObject{
		{Key: "db1/binlog/mysql-bin.000001"}, {Key: "db1/binlog/mysql-bin.000002"},
		{Key: "db1/binlog/mysql-bin.000003"}, {Key: "db1/binlog/readme"},
	}}
	del := &recordingDeleter{}
	n, err := PruneRemoteBinlogs(context.Background(), Destination{}, lister, del, "db1", "mysql-bin.000003:157")
	if err != nil {
		t.Fatalf("PruneRemoteBinlogs() error = %v", err)
	}
	if n != 2 || len(del.keys) != 2 || del.keys[1] != "db1/binlog/mysql-bin.000002" {
		t.Errorf("deleted %d %v, want files 1 and 2 only", n, del.keys)
	}
	if _, err := PruneRemoteBinlogs(context.Background(), Destination{}, lister, del, "db1", "0/3000060"); err == nil {
		t.Error("a Postgres LSN must be refused")
	}
}

func TestMySQLPITRRunner_RejectsBaseBackupWithoutCoordinate(t *testing.T) {
	st := &mysqlRunnerStore{bh: store.BaseBackupHistory{ID: "b1", Status: store.BackupStatusSucceeded, LSN: "0/3000060", StartedAt: "2026-10-22T10:00:00Z", FinishedAt: "2026-10-22T10:00:05Z"}}
	r := &MySQLPITRRunner{Store: st, Now: func() time.Time { return time.Unix(0, 0) }}
	err := r.RunPITRCloneRestore(context.Background(), "h1", "src", "new", "b1", store.EngineMySQL, "db-src", "db-new", "database/new", time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "no binlog position") {
		t.Fatalf("error = %v, want a missing binlog position error", err)
	}
	if st.finished != store.BackupStatusFailed {
		t.Errorf("history finished as %q, want failed", st.finished)
	}
}

func TestMySQLPITRRunner_RejectsTargetBeforeBaseBackupFinished(t *testing.T) {
	st := &mysqlRunnerStore{bh: store.BaseBackupHistory{ID: "b1", Status: store.BackupStatusSucceeded, LSN: "mysql-bin.000001:157", StartedAt: "2026-10-22T10:00:00Z", FinishedAt: "2026-10-22T10:00:05Z"}}
	r := &MySQLPITRRunner{Store: st}
	err := r.RunPITRCloneRestore(context.Background(), "h1", "src", "new", "b1", store.EngineMySQL, "db-src", "db-new", "database/new", time.Date(2026, 10, 22, 10, 0, 1, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "before base backup") {
		t.Fatalf("error = %v, want a target-before-backup error", err)
	}
}

func TestMySQLPITRRunner_WindowStartsWhenOldestBackupFinished(t *testing.T) {
	st := &mysqlRunnerStore{bh: store.BaseBackupHistory{ID: "b1", Status: store.BackupStatusSucceeded, LSN: "mysql-bin.000001:157", StartedAt: "2026-10-22T10:00:00Z", FinishedAt: "2026-10-22T10:00:05Z"}, has: true}
	r := &MySQLPITRRunner{Store: st, Runtime: execOnlyRuntime{exec: &fakeMySQLRuntime{}}}
	win, err := r.Window(context.Background(), "src", "db-src")
	if err != nil {
		t.Fatalf("Window() error = %v", err)
	}
	if !win.Start.Equal(time.Date(2026, 10, 22, 10, 0, 5, 0, time.UTC)) {
		t.Errorf("window start = %v, want the finish time", win.Start)
	}
	st.has = false
	if _, err := r.Window(context.Background(), "src", "db-src"); !errors.Is(err, ErrNoBaseBackup) {
		t.Errorf("Window() without a backup error = %v, want ErrNoBaseBackup", err)
	}
}

type mysqlRunnerStore struct {
	bh       store.BaseBackupHistory
	has      bool
	finished string
}

func (s *mysqlRunnerStore) GetBackupTarget(context.Context, string) (store.BackupTarget, error) {
	return store.BackupTarget{}, errors.New("no target")
}

func (s *mysqlRunnerStore) GetBaseBackupHistory(context.Context, string) (store.BaseBackupHistory, error) {
	return s.bh, nil
}

func (s *mysqlRunnerStore) GetOldestSucceededBaseBackup(context.Context, string) (store.BaseBackupHistory, bool, error) {
	return s.bh, s.has, nil
}

func (s *mysqlRunnerStore) StartPITRRestoreHistory(context.Context, store.PITRRestoreHistory) error {
	return nil
}

func (s *mysqlRunnerStore) FinishPITRRestoreHistory(_ context.Context, _, status, _, _ string) error {
	s.finished = status
	return nil
}

func (s *mysqlRunnerStore) GetConditions(context.Context, string) ([]reconcile.Condition, error) {
	return nil, nil
}

type execOnlyRuntime struct {
	docker.Runtime
	exec *fakeMySQLRuntime
}

func (r execOnlyRuntime) Exec(ctx context.Context, id string, cmd []string) (io.ReadCloser, error) {
	return r.exec.Exec(ctx, id, cmd)
}
