package backup

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestContainerRestorer_Restore_Postgres(t *testing.T) {
	rt := &fakeExecRuntime{content: "CREATE TABLE...\n"}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "-- pg_dump plain SQL output"
	if err := r.Restore(context.Background(), store.EnginePostgres, "db-mydb", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.gotInputContainer != "db-mydb" {
		t.Errorf("container = %q, want %q", rt.gotInputContainer, "db-mydb")
	}
	if rt.gotStdin != dump {
		t.Errorf("stdin = %q, want %q", rt.gotStdin, dump)
	}
	wantCmd := []string{"sh", "-c", `{ echo "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"; cat; } | psql --no-password -v ON_ERROR_STOP=1 --single-transaction -U "$POSTGRES_USER" "$POSTGRES_USER"`}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
}

func TestContainerRestorer_Restore_MySQL(t *testing.T) {
	rt := &fakeExecRuntime{content: "-- mysql restore output\n"}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "DROP TABLE IF EXISTS `t`;\nCREATE TABLE `t` (...);\n"
	if err := r.Restore(context.Background(), store.EngineMySQL, "db-mydb", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.gotInputContainer != "db-mydb" {
		t.Errorf("container = %q, want %q", rt.gotInputContainer, "db-mydb")
	}
	if rt.gotStdin != dump {
		t.Errorf("stdin = %q, want %q", rt.gotStdin, dump)
	}
	wantCmd := []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS ` + shBacktick + `$MYSQL_DATABASE` + shBacktick + `; CREATE DATABASE ` + shBacktick + `$MYSQL_DATABASE` + shBacktick + `;" && exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
}

func TestContainerRestorer_Restore_MongoDB(t *testing.T) {
	rt := &fakeExecRuntime{content: "-- mongorestore output\n"}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "mongodump archive bytes"
	if err := r.Restore(context.Background(), store.EngineMongoDB, "db-mydb", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.gotInputContainer != "db-mydb" {
		t.Errorf("container = %q, want %q", rt.gotInputContainer, "db-mydb")
	}
	if rt.gotStdin != dump {
		t.Errorf("stdin = %q, want %q", rt.gotStdin, dump)
	}
	wantCmd := []string{"sh", "-c", `mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'db.getMongo().getDBNames().forEach(function(n){if(n!=="admin"&&n!=="local"&&n!=="config"){db.getSiblingDB(n).dropDatabase()}})' && exec mongorestore --archive --drop --nsExclude "admin.*" --nsExclude "config.*" --nsExclude "local.*" --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
}

// TestContainerRestorer_Restore_Redis proves the write-then-stop-then-start
// sequence: the dump is streamed onto /data/dump.rdb via ExecWithInput
// while the container is still running, then the container is stopped and
// started (by ID, from InspectByName) so Redis reloads the RDB file at its
// next startup, in that exact order.
func TestContainerRestorer_Restore_Redis(t *testing.T) {
	rt := &fakeExecRuntime{
		inspectState: &docker.ContainerState{ID: "container-id-123"},
	}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "REDIS0011..."
	if err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.gotInputContainer != "db-cache" {
		t.Errorf("ExecWithInput container = %q, want %q", rt.gotInputContainer, "db-cache")
	}
	wantCmd := []string{"sh", "-c", "cat > /data/dump.rdb"}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("ExecWithInput cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
	if rt.gotStdin != dump {
		t.Errorf("stdin = %q, want %q", rt.gotStdin, dump)
	}
	if rt.gotStopID != "container-id-123" {
		t.Errorf("Stop id = %q, want %q", rt.gotStopID, "container-id-123")
	}
	if rt.gotStartID != "container-id-123" {
		t.Errorf("Start id = %q, want %q", rt.gotStartID, "container-id-123")
	}
	wantOrder := []string{"Exec", "ExecWithInput", "InspectByName", "Stop", "Start"}
	if !reflect.DeepEqual(rt.callOrder, wantOrder) {
		t.Errorf("call order = %v, want %v", rt.callOrder, wantOrder)
	}
	wantDisableSaveCmd := redisDisableAutoSaveCmd
	if !reflect.DeepEqual(rt.gotCmd, wantDisableSaveCmd) {
		t.Errorf("Exec cmd = %v, want %v", rt.gotCmd, wantDisableSaveCmd)
	}
}

func TestContainerRestorer_Restore_MariaDB(t *testing.T) {
	rt := &fakeExecRuntime{content: "-- mariadb restore output\n"}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "DROP TABLE IF EXISTS `t`;\nCREATE TABLE `t` (...);\n"
	if err := r.Restore(context.Background(), store.EngineMariaDB, "db-mydb", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.gotInputContainer != "db-mydb" {
		t.Errorf("container = %q, want %q", rt.gotInputContainer, "db-mydb")
	}
	if rt.gotStdin != dump {
		t.Errorf("stdin = %q, want %q", rt.gotStdin, dump)
	}
	wantCmd := []string{"sh", "-c", `mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS ` + shBacktick + `$MARIADB_DATABASE` + shBacktick + `; CREATE DATABASE ` + shBacktick + `$MARIADB_DATABASE` + shBacktick + `;" && exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" "$MARIADB_DATABASE"`}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
}

// TestContainerRestorer_Restore_KeyDB mirrors
// TestContainerRestorer_Restore_Redis: same write-then-stop-then-start
// sequence, the only real difference being keydb-cli in place of
// redis-cli for the disable-save-points step (see keydbCLIBin).
func TestContainerRestorer_Restore_KeyDB(t *testing.T) {
	rt := &fakeExecRuntime{
		inspectState: &docker.ContainerState{ID: "container-id-456"},
	}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "REDIS0011..."
	if err := r.Restore(context.Background(), store.EngineKeyDB, "db-cache", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	wantCmd := []string{"sh", "-c", "cat > /data/dump.rdb"}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("ExecWithInput cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
	wantDisableSaveCmd := []string{"keydb-cli", "CONFIG", "SET", "save", ""}
	if !reflect.DeepEqual(rt.gotCmd, wantDisableSaveCmd) {
		t.Errorf("Exec cmd = %v, want %v", rt.gotCmd, wantDisableSaveCmd)
	}
	wantOrder := []string{"Exec", "ExecWithInput", "InspectByName", "Stop", "Start"}
	if !reflect.DeepEqual(rt.callOrder, wantOrder) {
		t.Errorf("call order = %v, want %v", rt.callOrder, wantOrder)
	}
}

// TestContainerRestorer_Restore_Redis_DisableSaveError proves a failure
// disabling save points (the Exec call) aborts before the RDB is even
// written, and before Stop/Start run: this call has to happen and
// succeed first, see restoreRedisLike's own doc comment for why (Redis's
// own SIGTERM-triggered save would otherwise overwrite the restored RDB
// file with the live pre-restore state on the way down).
func TestContainerRestorer_Restore_Redis_DisableSaveError(t *testing.T) {
	wantErr := errors.New("exec: command exited 1")
	rt := &fakeExecRuntime{err: wantErr, inspectState: &docker.ContainerState{ID: "container-id-123"}}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the wrapped Exec error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Restore() error = %v, want it to wrap %v", err, wantErr)
	}
	if !reflect.DeepEqual(rt.callOrder, []string{"Exec"}) {
		t.Errorf("call order = %v, want only Exec, ExecWithInput/Stop/Start must not run", rt.callOrder)
	}
}

func TestContainerRestorer_Restore_Redis_ExecWithInputError(t *testing.T) {
	wantErr := errors.New("exec: command exited 1")
	rt := &fakeExecRuntime{execWithInputErr: wantErr, inspectState: &docker.ContainerState{ID: "container-id-123"}}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the wrapped ExecWithInput error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Restore() error = %v, want it to wrap %v", err, wantErr)
	}
	if !reflect.DeepEqual(rt.callOrder, []string{"Exec", "ExecWithInput"}) {
		t.Errorf("call order = %v, want Exec then ExecWithInput only, Stop/Start must not run", rt.callOrder)
	}
}

func TestContainerRestorer_Restore_Redis_InspectError(t *testing.T) {
	wantErr := errors.New("inspect: connection refused")
	rt := &fakeExecRuntime{inspectErr: wantErr}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the wrapped InspectByName error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Restore() error = %v, want it to wrap %v", err, wantErr)
	}
	if !reflect.DeepEqual(rt.callOrder, []string{"Exec", "ExecWithInput", "InspectByName"}) {
		t.Errorf("call order = %v, want Exec, ExecWithInput, InspectByName only, Stop/Start must not run", rt.callOrder)
	}
}

func TestContainerRestorer_Restore_Redis_NotFound(t *testing.T) {
	rt := &fakeExecRuntime{inspectState: nil}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want an error when the restore target container doesn't exist")
	}
	if !reflect.DeepEqual(rt.callOrder, []string{"Exec", "ExecWithInput", "InspectByName"}) {
		t.Errorf("call order = %v, want Exec, ExecWithInput, InspectByName only, Stop/Start must not run", rt.callOrder)
	}
}

func TestContainerRestorer_Restore_Redis_StopError(t *testing.T) {
	wantErr := errors.New("stop: timeout")
	rt := &fakeExecRuntime{stopErr: wantErr, inspectState: &docker.ContainerState{ID: "container-id-123"}}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the wrapped Stop error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Restore() error = %v, want it to wrap %v", err, wantErr)
	}
	wantOrder := []string{"Exec", "ExecWithInput", "InspectByName", "Stop"}
	if !reflect.DeepEqual(rt.callOrder, wantOrder) {
		t.Errorf("call order = %v, want %v, Start must not run after a failed Stop", rt.callOrder, wantOrder)
	}
}

func TestContainerRestorer_Restore_Redis_StartError(t *testing.T) {
	wantErr := errors.New("start: no such container")
	rt := &fakeExecRuntime{startErr: wantErr, inspectState: &docker.ContainerState{ID: "container-id-123"}}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the wrapped Start error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Restore() error = %v, want it to wrap %v", err, wantErr)
	}
	wantOrder := []string{"Exec", "ExecWithInput", "InspectByName", "Stop", "Start"}
	if !reflect.DeepEqual(rt.callOrder, wantOrder) {
		t.Errorf("call order = %v, want %v", rt.callOrder, wantOrder)
	}
}

// TestContainerRestorer_Restore_Dragonfly mirrors
// TestContainerRestorer_Restore_Redis's write-then-stop-then-start
// sequence, but the disable-auto-save step clears dbfilename rather than
// save (see dragonflyDisableAutoSaveCmd's own doc comment).
func TestContainerRestorer_Restore_Dragonfly(t *testing.T) {
	rt := &fakeExecRuntime{
		inspectState: &docker.ContainerState{ID: "container-id-789"},
	}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "REDIS0011..."
	if err := r.Restore(context.Background(), store.EngineDragonfly, "db-cache", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	wantCmd := []string{"sh", "-c", "cat > /data/dump.rdb"}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("ExecWithInput cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
	if rt.gotStdin != dump {
		t.Errorf("stdin = %q, want %q", rt.gotStdin, dump)
	}
	wantDisableSaveCmd := []string{"redis-cli", "CONFIG", "SET", "dbfilename", ""}
	if !reflect.DeepEqual(rt.gotCmd, wantDisableSaveCmd) {
		t.Errorf("Exec cmd = %v, want %v", rt.gotCmd, wantDisableSaveCmd)
	}
	wantOrder := []string{"Exec", "ExecWithInput", "InspectByName", "Stop", "Start"}
	if !reflect.DeepEqual(rt.callOrder, wantOrder) {
		t.Errorf("call order = %v, want %v", rt.callOrder, wantOrder)
	}
	if rt.gotStopID != "container-id-789" {
		t.Errorf("Stop id = %q, want %q", rt.gotStopID, "container-id-789")
	}
	if rt.gotStartID != "container-id-789" {
		t.Errorf("Start id = %q, want %q", rt.gotStartID, "container-id-789")
	}
}

func TestContainerRestorer_Restore_ClickHouse(t *testing.T) {
	rt := &fakeExecRuntime{content: "-- clickhouse restore output\n"}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	dump := "CREATE TABLE t (...) ENGINE = MergeTree ORDER BY id;\nINSERT INTO t (id) VALUES (1);\n"
	if err := r.Restore(context.Background(), store.EngineClickHouse, "db-mydb", strings.NewReader(dump)); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.gotInputContainer != "db-mydb" {
		t.Errorf("container = %q, want %q", rt.gotInputContainer, "db-mydb")
	}
	if rt.gotStdin != dump {
		t.Errorf("stdin = %q, want %q", rt.gotStdin, dump)
	}
	wantCmd := []string{"sh", "-c", `clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "DROP DATABASE IF EXISTS ` + shBacktick + `$CLICKHOUSE_DB` + shBacktick + `; CREATE DATABASE ` + shBacktick + `$CLICKHOUSE_DB` + shBacktick + `" && exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"`}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
}

func TestContainerRestorer_Restore_UnknownEngine(t *testing.T) {
	rt := &fakeExecRuntime{}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), "cassandra", "db-mydb", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want an error for an unrecognized engine")
	}
	if rt.execWithInputCalls != 0 {
		t.Errorf("ExecWithInput called %d times for an unrecognized engine, want 0", rt.execWithInputCalls)
	}
}

func TestContainerRestorer_Restore_ExecError(t *testing.T) {
	wantErr := errors.New("exec: command exited 1")
	rt := &fakeExecRuntime{err: wantErr}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EnginePostgres, "db-mydb", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the wrapped ExecWithInput error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Restore() error = %v, want it to wrap %v", err, wantErr)
	}
}

// fakeExecReadCloserErr is a ReadCloser whose Read always fails, used to
// prove Restore surfaces a failure discovered only while draining the
// command's own stdout (e.g. a non-zero exit reported the way
// docker.Client's streamExecOutput reports one, as a trailing Read
// error) rather than only checking ExecWithInput's own immediate return.
type fakeExecReadCloserErr struct{ err error }

func (f fakeExecReadCloserErr) Read([]byte) (int, error) { return 0, f.err }
func (f fakeExecReadCloserErr) Close() error             { return nil }

func TestContainerRestorer_Restore_StreamErrorAfterExecSucceeds(t *testing.T) {
	wantErr := errors.New("docker: exec ... exited 1: ERROR: syntax error")
	rt := &execWithInputStreamErrRuntime{err: wantErr}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Millisecond, ReadyInterval: time.Millisecond}

	err := r.Restore(context.Background(), store.EnginePostgres, "db-mydb", strings.NewReader("garbage sql"))
	if err == nil {
		t.Fatal("Restore() error = nil, want the stream error surfaced")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Restore() error = %v, want it to wrap %v", err, wantErr)
	}
}

// execWithInputStreamErrRuntime is a minimal docker.Runtime fake,
// distinct from fakeExecRuntime (dump_test.go), whose ExecWithInput
// succeeds (returns a non-nil stream) but that stream fails on Read: the
// shape a real exec session takes when the command itself runs and later
// exits non-zero, as opposed to ExecWithInput failing to even start the
// exec session at all (fakeExecRuntime.err covers that case already).
type execWithInputStreamErrRuntime struct {
	fakeExecRuntime
	err error
}

func (f *execWithInputStreamErrRuntime) ExecWithInput(_ context.Context, _ string, _ []string, stdin io.Reader) (io.ReadCloser, error) {
	_, _ = io.Copy(io.Discard, stdin)
	return fakeExecReadCloserErr{err: f.err}, nil
}

type flakyReadyRuntime struct {
	*fakeExecRuntime
	failures int
}

func (f *flakyReadyRuntime) Exec(ctx context.Context, id string, cmd []string) (io.ReadCloser, error) {
	if f.failures > 0 {
		f.failures--
		return nil, errors.New("pg_isready: no response")
	}
	return f.fakeExecRuntime.Exec(ctx, id, cmd)
}

func TestContainerRestorer_WaitsForServerBeforeRestoring(t *testing.T) {
	inner := &fakeExecRuntime{}
	rt := &flakyReadyRuntime{fakeExecRuntime: inner, failures: 3}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: time.Second, ReadyInterval: time.Millisecond}

	if err := r.Restore(context.Background(), store.EnginePostgres, "db-new", strings.NewReader("sql")); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if rt.failures != 0 || inner.execWithInputCalls != 1 {
		t.Fatalf("failures left = %d, restore calls = %d, want the restore to run after the probe succeeded", rt.failures, inner.execWithInputCalls)
	}
}

func TestContainerRestorer_GivesUpWhenServerNeverComesUp(t *testing.T) {
	rt := &flakyReadyRuntime{fakeExecRuntime: &fakeExecRuntime{}, failures: 1 << 20}
	r := &ContainerRestorer{Runtime: rt, ReadyTimeout: 20 * time.Millisecond, ReadyInterval: time.Millisecond}
	if err := r.Restore(context.Background(), store.EngineMySQL, "db-new", strings.NewReader("sql")); err == nil {
		t.Fatal("Restore succeeded against a server that never accepted connections")
	}
	if rt.execWithInputCalls != 0 {
		t.Fatal("restore ran against a server that was not ready")
	}
}
