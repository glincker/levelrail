package backup

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestContainerRestorer_Restore_Postgres(t *testing.T) {
	rt := &fakeExecRuntime{content: "CREATE TABLE...\n"}
	r := &ContainerRestorer{Runtime: rt}

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
	wantCmd := []string{"sh", "-c", `psql --no-password -U "$POSTGRES_USER" "$POSTGRES_USER" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" && exec psql --no-password -U "$POSTGRES_USER" "$POSTGRES_USER"`}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
}

func TestContainerRestorer_Restore_MySQL(t *testing.T) {
	rt := &fakeExecRuntime{content: "-- mysql restore output\n"}
	r := &ContainerRestorer{Runtime: rt}

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
	wantCmd := []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS $MYSQL_DATABASE; CREATE DATABASE $MYSQL_DATABASE;" && exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`}
	if !reflect.DeepEqual(rt.gotInputCmd, wantCmd) {
		t.Errorf("cmd = %v, want %v", rt.gotInputCmd, wantCmd)
	}
}

// TestContainerRestorer_Restore_Redis_Unsupported proves Redis restore
// fails loudly and identifiably (ErrRedisRestoreNotSupported), never
// silently succeeding or silently no-oping: an RDB snapshot piped into a
// live Redis the way this method pipes SQL into psql/mysql would not do
// anything close to a restore, so the honest behavior is a clear,
// checkable error, not an attempt.
func TestContainerRestorer_Restore_Redis_Unsupported(t *testing.T) {
	rt := &fakeExecRuntime{}
	r := &ContainerRestorer{Runtime: rt}

	err := r.Restore(context.Background(), store.EngineRedis, "db-cache", strings.NewReader("REDIS0011..."))
	if err == nil {
		t.Fatal("Restore() error = nil, want ErrRedisRestoreNotSupported for redis")
	}
	if !errors.Is(err, ErrRedisRestoreNotSupported) {
		t.Errorf("Restore() error = %v, want it to wrap ErrRedisRestoreNotSupported", err)
	}
	if rt.execWithInputCalls != 0 {
		t.Errorf("ExecWithInput called %d times for redis, want 0", rt.execWithInputCalls)
	}
}

func TestContainerRestorer_Restore_UnknownEngine(t *testing.T) {
	rt := &fakeExecRuntime{}
	r := &ContainerRestorer{Runtime: rt}

	err := r.Restore(context.Background(), "mongodb", "db-mydb", strings.NewReader("x"))
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
	r := &ContainerRestorer{Runtime: rt}

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
	r := &ContainerRestorer{Runtime: rt}

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
