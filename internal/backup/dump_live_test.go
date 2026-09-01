package backup

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// liveRuntime returns a docker.Client connected to a real daemon, or
// skips the test cleanly if none is reachable, the same pattern
// internal/docker's own liveClient establishes; duplicated here rather
// than exported from that package for one test file's sake, matching
// how test/e2e's own helpers already accept the small duplication over
// growing internal/docker's exported surface for tests alone.
func liveRuntime(t *testing.T) *docker.Client {
	t.Helper()
	c, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("closing client: %v", err)
		}
	})
	return c
}

func removeContainerIfExists(ctx context.Context, t *testing.T, rt docker.Runtime, name string) {
	t.Helper()
	state, err := rt.InspectByName(ctx, name)
	if err != nil || state == nil {
		return
	}
	_ = rt.Remove(ctx, state.ID, true)
}

// waitReady re-runs probe via rt.Exec against containerName until it
// exits zero or timeout elapses, the same "poll a real exec'd command
// until the daemon inside the container is actually ready to serve"
// approach a health check would use, needed here because Create/Start
// returning success only means the container process started, not that
// the database engine has finished initializing its data directory and
// is accepting real connections yet.
func waitReady(ctx context.Context, t *testing.T, rt docker.Runtime, containerName string, probe []string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		rc, err := rt.Exec(ctx, containerName, probe)
		if err == nil {
			_, readErr := io.Copy(io.Discard, rc)
			_ = rc.Close()
			if readErr == nil {
				return
			}
			lastErr = readErr
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("container %q never became ready within %v: %v", containerName, timeout, lastErr)
}

// waitMySQLReady is waitReady plus a second confirming probe a moment
// later, specifically for MySQL: the official mysql:8 image's entrypoint
// runs a temporary, socket-only mysqld to initialize the data directory
// and run init scripts (creating MYSQL_DATABASE/MYSQL_USER), then shuts
// that instance down and starts the real, permanent one on the same
// socket path. A single successful probe cannot tell those two servers
// apart: it is possible, and was observed causing real CI failures in
// TestContainerDumper_Dump_MySQL_Live and
// TestContainerRestorer_Restore_MySQL_Live (never reproduced locally,
// where the temporary instance's own window is apparently too narrow to
// usually land a probe in, but reliably hit on CI's more loaded/slower
// daemon), for waitReady's own probe to succeed against the temporary
// instance moments before it exits, with the very next real command
// (the seed step) then failing to connect at all or losing its
// connection mid-query as the handoff to the permanent instance happens
// underneath it.
//
// Requiring the same probe to succeed twice, with a real gap in between,
// is what actually distinguishes the two cases: the temporary instance
// is gone for good once it shuts down, so a second probe shortly after
// a first success can only succeed if the server answering it is the
// stable, permanent one. Postgres and Redis have no equivalent two-phase
// startup (see postgresDumpCmd's and redisDumpCmd's own doc comments;
// neither image restarts its own server process during initialization
// the way MySQL's entrypoint does), so this stays MySQL-specific rather
// than folded into waitReady itself for every engine.
func waitMySQLReady(ctx context.Context, t *testing.T, rt docker.Runtime, containerName string, probe []string, timeout time.Duration) {
	t.Helper()
	waitReady(ctx, t, rt, containerName, probe, timeout)
	time.Sleep(2 * time.Second)
	waitReady(ctx, t, rt, containerName, probe, timeout)
}

// TestContainerDumper_Dump_Postgres_Live proves the entire Postgres dump
// path end to end against a real Postgres container, provisioned with
// exactly the environment internal/reconcile/database's Controller
// actually sets (POSTGRES_USER/POSTGRES_PASSWORD only, no POSTGRES_DB),
// not a convenient test-only shape: seeds one real row through a real
// connection, runs ContainerDumper.Dump, and checks the seeded value is
// actually present in the dumped bytes. This is the strongest evidence
// available that postgresDumpCmd's reasoning (trust auth over the local
// socket, POSTGRES_DB defaulting to POSTGRES_USER) is correct against a
// real image, not just correct on paper.
func TestContainerDumper_Dump_Postgres_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-backup-postgres"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "postgres:16",
		Env: map[string]string{
			"POSTGRES_USER":     "leveltest",
			"POSTGRES_PASSWORD": "leveltestpass",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Deliberately not pg_isready: the official Postgres entrypoint runs
	// a temporary internal server to perform initdb (creating the
	// leveltest role and its same-named database) before stopping it and
	// starting the real one, and pg_isready happily reports ready
	// against that temporary server too. A real query against the
	// actual target database is the only probe that can't be fooled by
	// that intermediate state, which a first attempt at this test caught
	// directly: pg_isready succeeded while "database leveltest does not
	// exist" was still the real error a moment later.
	waitReady(ctx, t, rt, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)

	seedMarker := "levelrail-live-marker-8f21c4"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`psql -U leveltest -d leveltest -c "CREATE TABLE backup_probe (val text); INSERT INTO backup_probe VALUES ('` + seedMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	rc, err := d.Dump(ctx, store.EnginePostgres, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	dump, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}

	if !strings.Contains(string(dump), seedMarker) {
		t.Errorf("dump does not contain the seeded marker %q; dump was %d bytes", seedMarker, len(dump))
	}
	if !strings.Contains(string(dump), "backup_probe") {
		t.Errorf("dump does not mention the seeded table backup_probe; dump was %d bytes", len(dump))
	}
}

// TestContainerDumper_Dump_MySQL_Live is Postgres's live test's exact
// counterpart for MySQL: a real container provisioned the way
// internal/reconcile/database's Controller actually provisions one
// (MYSQL_ROOT_PASSWORD, MYSQL_DATABASE, MYSQL_USER, MYSQL_PASSWORD all
// set, per WithMySQLCredentials), seeded with one real row through a
// real connection, dumped through ContainerDumper, and checked for that
// row's presence in the output.
func TestContainerDumper_Dump_MySQL_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-backup-mysql"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "mysql:8",
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": "leveltestroot",
			"MYSQL_DATABASE":      "leveltest",
			"MYSQL_USER":          "leveltest",
			"MYSQL_PASSWORD":      "leveltestpass",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// A real query against the target database, the same reasoning as
	// Postgres's own probe: mysqld also runs a temporary init pass
	// before the real server comes up, so only a query that actually
	// touches leveltest proves the database this test needs is there.
	waitMySQLReady(ctx, t, rt, name, []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "SELECT 1"`}, 60*time.Second)

	seedMarker := "levelrail-live-marker-mysql-3d9a"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "CREATE TABLE backup_probe (val varchar(64)); INSERT INTO backup_probe VALUES ('` + seedMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	rc, err := d.Dump(ctx, store.EngineMySQL, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	dump, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}

	if !strings.Contains(string(dump), seedMarker) {
		t.Errorf("dump does not contain the seeded marker %q; dump was %d bytes", seedMarker, len(dump))
	}
	if !strings.Contains(string(dump), "backup_probe") {
		t.Errorf("dump does not mention the seeded table backup_probe; dump was %d bytes", len(dump))
	}
}

// TestContainerDumper_Dump_Redis_Live proves redisDumpCmd against a real
// Redis container: sets one real key through a real connection, dumps
// through ContainerDumper, and checks the result both structurally (the
// RDB magic header every well-formed RDB file starts with, the literal
// bytes "REDIS" followed by a four-digit version) and for the seeded
// key/value pair's presence in the bytes.
func TestContainerDumper_Dump_Redis_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-backup-redis"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "redis:7",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitReady(ctx, t, rt, name, []string{"redis-cli", "PING"}, 15*time.Second)

	seed, err := rt.Exec(ctx, name, []string{"redis-cli", "SET", "backup_probe", "levelrail-live-marker-redis-71ae"})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	rc, err := d.Dump(ctx, store.EngineRedis, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	dump, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}

	if len(dump) < 9 || string(dump[:5]) != "REDIS" {
		t.Fatalf("dump does not start with the RDB magic header \"REDIS\"; got %q (first bytes: %v)", string(dump[:min(9, len(dump))]), dump[:min(9, len(dump))])
	}
	// Short of hand-rolling a real RDB decoder for one test, a
	// substring check on the raw bytes is the practical proof available:
	// RDB stores short string values like these largely as their literal
	// bytes rather than compressed or binary-encoded, so both the key
	// name and the value marker landing in the byte stream is real
	// evidence the seeded pair made it into the snapshot, on top of the
	// magic-header check above already confirming this is a well-formed
	// RDB file and not arbitrary bytes.
	if !strings.Contains(string(dump), "backup_probe") || !strings.Contains(string(dump), "levelrail-live-marker-redis-71ae") {
		t.Errorf("dump does not contain the seeded key/value; dump was %d bytes", len(dump))
	}
}
