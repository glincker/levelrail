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

// fakeExecRuntime is a hand-written fake docker.Runtime, the same
// convention every other package's tests in this codebase already
// follow, narrowed to what ContainerDumper actually exercises: Exec
// alone. Every other method is a compile-satisfying stub, unused by
// this file's tests.
type fakeExecRuntime struct {
	gotContainer string
	gotCmd       []string
	execCalls    int

	content string
	err     error

	// ExecWithInput's own recorded call, kept separate from Exec's fields
	// above rather than shared: restore_test.go's tests need to assert on
	// what was passed to ExecWithInput specifically (including the stdin
	// bytes), and reusing Exec's fields would make a test that calls both
	// methods on the same fake ambiguous about which call it's checking.
	gotInputContainer  string
	gotInputCmd        []string
	gotStdin           string
	execWithInputCalls int
	// execWithInputErr, when set, fails ExecWithInput specifically without
	// also failing Exec (err above fails both, since Redis restore now
	// calls Exec for CONFIG SET before ExecWithInput for the RDB write,
	// and some restore_test.go cases need to fail one without the other).
	execWithInputErr error

	// Redis restore path: InspectByName/Stop/Start recorded calls plus a
	// callOrder log so restore_test.go can assert the exact
	// write-then-stop-then-start sequence, not just that each call
	// happened.
	inspectState *docker.ContainerState
	inspectErr   error
	stopErr      error
	startErr     error
	gotStopID    string
	gotStartID   string
	callOrder    []string
}

func (f *fakeExecRuntime) Exec(_ context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	f.execCalls++
	f.callOrder = append(f.callOrder, "Exec")
	f.gotContainer = containerID
	f.gotCmd = cmd
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

// ExecWithInput reads stdin to completion before returning, the same way
// the real docker.Client implementation's stdin-copy goroutine always
// finishes writing before streamExecOutput's stdout side reaches EOF for
// a well-behaved command: restore_test.go's tests depend on gotStdin
// reflecting the whole dump, not a partial read racing the fake's own
// return.
func (f *fakeExecRuntime) ExecWithInput(_ context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	f.execWithInputCalls++
	f.callOrder = append(f.callOrder, "ExecWithInput")
	f.gotInputContainer = containerID
	f.gotInputCmd = cmd
	stdinBytes, readErr := io.ReadAll(stdin)
	f.gotStdin = string(stdinBytes)
	if readErr != nil {
		return nil, readErr
	}
	if f.execWithInputErr != nil {
		return nil, f.execWithInputErr
	}
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

func (f *fakeExecRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	f.callOrder = append(f.callOrder, "InspectByName")
	return f.inspectState, f.inspectErr
}
func (f *fakeExecRuntime) Create(context.Context, docker.ContainerSpec) (string, error) {
	return "", nil
}
func (f *fakeExecRuntime) Start(_ context.Context, id string) error {
	f.callOrder = append(f.callOrder, "Start")
	f.gotStartID = id
	return f.startErr
}
func (f *fakeExecRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakeExecRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeExecRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeExecRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	f.callOrder = append(f.callOrder, "Stop")
	f.gotStopID = id
	return f.stopErr
}
func (f *fakeExecRuntime) Remove(context.Context, string, bool) error { return nil }
func (f *fakeExecRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *fakeExecRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *fakeExecRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeExecRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakeExecRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

func TestContainerDumper_Dump(t *testing.T) {
	cases := []struct {
		name        string
		engine      string
		containerID string
		content     string
		wantCmd     []string
	}{
		{
			name:        "Postgres",
			engine:      store.EnginePostgres,
			containerID: "db-mydb",
			content:     "postgres-dump-bytes",
			wantCmd:     []string{"sh", "-c", `exec pg_dump --no-password -U "$POSTGRES_USER" "$POSTGRES_USER"`},
		},
		{
			name:        "MySQL",
			engine:      store.EngineMySQL,
			containerID: "db-mydb",
			content:     "mysql-dump-bytes",
			wantCmd:     []string{"sh", "-c", `exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`},
		},
		{
			name:        "MongoDB",
			engine:      store.EngineMongoDB,
			containerID: "db-mydb",
			content:     "mongo-dump-bytes",
			wantCmd:     []string{"sh", "-c", `exec mongodump --archive --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`},
		},
		{
			name:        "Redis",
			engine:      store.EngineRedis,
			containerID: "db-cache",
			content:     "rdb-bytes",
			wantCmd:     []string{"redis-cli", "--rdb", "-"},
		},
		{
			name:        "MariaDB",
			engine:      store.EngineMariaDB,
			containerID: "db-mydb",
			content:     "mariadb-dump-bytes",
			wantCmd:     []string{"sh", "-c", `exec mariadb-dump -uroot -p"$MARIADB_ROOT_PASSWORD" "$MARIADB_DATABASE"`},
		},
		{
			name:        "KeyDB",
			engine:      store.EngineKeyDB,
			containerID: "db-cache",
			content:     "rdb-bytes",
			wantCmd:     []string{"keydb-cli", "--rdb", "-"},
		},
		{
			name:        "Dragonfly",
			engine:      store.EngineDragonfly,
			containerID: "db-cache",
			content:     "rdb-bytes",
			wantCmd:     []string{"sh", "-c", "redis-cli SAVE RDB dump.rdb > /dev/null && exec cat /data/dump.rdb"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &fakeExecRuntime{content: tc.content}
			d := &ContainerDumper{Runtime: rt}

			rc, err := d.Dump(context.Background(), tc.engine, tc.containerID)
			if err != nil {
				t.Fatalf("Dump() error = %v", err)
			}
			defer func() { _ = rc.Close() }()

			if rt.gotContainer != tc.containerID {
				t.Errorf("container = %q, want %q", rt.gotContainer, tc.containerID)
			}
			if !reflect.DeepEqual(rt.gotCmd, tc.wantCmd) {
				t.Errorf("cmd = %v, want %v", rt.gotCmd, tc.wantCmd)
			}

			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if string(got) != tc.content {
				t.Errorf("content = %q, want %q", got, tc.content)
			}
		})
	}
}

func TestContainerDumper_Dump_ClickHouse(t *testing.T) {
	rt := &fakeExecRuntime{content: "CREATE TABLE...;\nINSERT INTO t...;\n"}
	d := &ContainerDumper{Runtime: rt}

	rc, err := d.Dump(context.Background(), store.EngineClickHouse, "db-mydb")
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	if rt.gotContainer != "db-mydb" {
		t.Errorf("container = %q, want %q", rt.gotContainer, "db-mydb")
	}
	if got, want := rt.gotCmd[0], "sh"; got != want {
		t.Errorf("cmd[0] = %q, want %q", got, want)
	}
	script := rt.gotCmd[2]
	for _, want := range []string{
		`--query "SHOW TABLES FROM $CLICKHOUSE_DB"`,
		`--query "SHOW CREATE TABLE $CLICKHOUSE_DB.$t" --format TSVRaw`,
		`SETTINGS output_format_sql_insert_table_name = '$t' FORMAT SQLInsert`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("dump script missing %q, got %q", want, script)
		}
	}

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "CREATE TABLE...;\nINSERT INTO t...;\n" {
		t.Errorf("content = %q, want dump content unchanged", got)
	}
}

func TestContainerDumper_Dump_UnknownEngine(t *testing.T) {
	rt := &fakeExecRuntime{}
	d := &ContainerDumper{Runtime: rt}

	_, err := d.Dump(context.Background(), "cassandra", "db-mydb")
	if err == nil {
		t.Fatal("Dump() error = nil, want an error for an unrecognized engine")
	}
	if rt.execCalls != 0 {
		t.Errorf("Exec called %d times for an unrecognized engine, want 0", rt.execCalls)
	}
}

func TestContainerDumper_Dump_ExecError(t *testing.T) {
	wantErr := errors.New("exec: command exited 1")
	rt := &fakeExecRuntime{err: wantErr}
	d := &ContainerDumper{Runtime: rt}

	_, err := d.Dump(context.Background(), store.EnginePostgres, "db-mydb")
	if err == nil {
		t.Fatal("Dump() error = nil, want the wrapped Exec error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Dump() error = %v, want it to wrap %v", err, wantErr)
	}
}
