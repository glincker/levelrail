package backup

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestContainerRestorer_Restore_Postgres_Live is the strongest proof
// available that a Postgres restore actually restores, run against a
// real container: seed a marker row, dump it (ContainerDumper, already
// proven live by dump_live_test.go), overwrite that same table with
// different data to prove a real change happened, restore from the
// captured dump (ContainerRestorer, this file), then query the live
// database directly and check the original marker is back and the
// overwriting change is gone. Not a mock or a fake anywhere in the
// path between the dump bytes and the container psql actually runs
// against.
//
// This also exercises the drop-and-replace decision postgresRestoreCmd's
// own doc comment describes end to end: the overwritten data has to be
// genuinely gone afterward, not merged with the restored rows, for this
// test to pass, which a merge-shaped restore would fail.
func TestContainerRestorer_Restore_Postgres_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-postgres"
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
	waitReady(ctx, t, rt, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)

	originalMarker := "levelrail-restore-live-original-6f1a"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`psql -U leveltest -d leveltest -c "CREATE TABLE restore_probe (val text); INSERT INTO restore_probe VALUES ('` + originalMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	// Capture the dump while the original data is still the only data
	// present, exactly the artifact a real backup would have produced at
	// this point.
	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EnginePostgres, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	// Overwrite: drop the seeded row and table, replace with something
	// else entirely, proving a real, distinguishable change happened
	// before restore runs.
	overwriteMarker := "levelrail-restore-live-overwritten-9c3d"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`psql -U leveltest -d leveltest -c "DROP TABLE restore_probe; CREATE TABLE unrelated_table (val text); INSERT INTO unrelated_table VALUES ('` + overwriteMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	// Restore from the captured dump.
	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EnginePostgres, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify directly against the live database, not against the dump
	// bytes: restore_probe (with the original marker) must be back, and
	// unrelated_table (the overwrite) must be gone, proving drop-and-
	// replace, not a merge that would have left both tables present.
	check, err := rt.Exec(ctx, name, []string{"sh", "-c", `psql -U leveltest -d leveltest -t -c "SELECT val FROM restore_probe;"`})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"sh", "-c", `psql -U leveltest -d leveltest -t -c "SELECT to_regclass('unrelated_table');"`})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if strings.Contains(string(goneOut), "unrelated_table") {
		t.Errorf("unrelated_table still exists after restore (output %q), want the overwrite fully replaced, not merged", goneOut)
	}
}

// TestContainerRestorer_Restore_MySQL_Live is
// TestContainerRestorer_Restore_Postgres_Live's exact MySQL counterpart:
// real container, real dump, a real overwriting change, a real restore,
// and a direct query against the live database afterward proving the
// original data is back. Unlike Postgres, mysqlRestoreCmd adds no
// explicit drop/recreate step (see its own doc comment): this test is
// what actually proves mysqldump's own default DROP TABLE IF EXISTS
// behavior is sufficient on its own, not just a claim taken on faith
// from reading mysqldump's documentation.
func TestContainerRestorer_Restore_MySQL_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-mysql"
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
	waitMySQLReady(ctx, t, rt, name, []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "SELECT 1"`}, 60*time.Second)

	originalMarker := "levelrail-restore-live-original-mysql-2e7b"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "CREATE TABLE restore_probe (val varchar(128)); INSERT INTO restore_probe VALUES ('` + originalMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineMySQL, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	overwriteMarker := "levelrail-restore-live-overwritten-mysql-8a4f"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "DROP TABLE restore_probe; CREATE TABLE unrelated_table (val varchar(128)); INSERT INTO unrelated_table VALUES ('` + overwriteMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineMySQL, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	check, err := rt.Exec(ctx, name, []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -N -e "SELECT val FROM restore_probe;"`})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -N -e "SHOW TABLES LIKE 'unrelated_table';"`})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if strings.Contains(string(goneOut), "unrelated_table") {
		t.Errorf("unrelated_table still exists after restore (output %q), want the overwrite fully replaced", goneOut)
	}
}

// TestContainerRestorer_Restore_Redis_Live proves the write-then-stop-then-
// start restore path against a real Redis container: seed a key, dump it
// via ContainerDumper (already proven live by
// TestContainerDumper_Dump_Redis_Live), overwrite that key and add another,
// restore from the captured RDB, then query the live server directly and
// confirm the original key is back and the overwrite is gone, proving the
// dump.rdb write plus stop/start reload actually took effect, not just that
// Restore() returned nil.
func TestContainerRestorer_Restore_Redis_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-redis"
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

	originalMarker := "levelrail-restore-live-original-redis-4d6e"
	seed, err := rt.Exec(ctx, name, []string{"redis-cli", "SET", "restore_probe", originalMarker})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineRedis, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	// Overwrite: delete the seeded key and set a different one, proving a
	// real, distinguishable change happened before restore runs.
	overwriteMarker := "levelrail-restore-live-overwritten-redis-9f2c"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		"redis-cli DEL restore_probe && redis-cli SET unrelated_key " + overwriteMarker,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineRedis, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Restore stops and restarts the container; give redis-server a moment
	// to finish loading dump.rdb and accept connections again before
	// probing it, the same reasoning waitReady's own polling loop exists
	// for after Create/Start.
	waitReady(ctx, t, rt, name, []string{"redis-cli", "PING"}, 15*time.Second)

	check, err := rt.Exec(ctx, name, []string{"redis-cli", "GET", "restore_probe"})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"redis-cli", "EXISTS", "unrelated_key"})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if !strings.Contains(string(goneOut), "0") {
		t.Errorf("unrelated_key EXISTS = %q, want \"0\" (gone), restore should have replaced it with the pre-overwrite snapshot", goneOut)
	}
}

// waitMongoReady mirrors waitMySQLReady: the official mongo image's
// entrypoint also runs a temporary, auth-disabled server to create the
// root user before shutting it down and starting the real, --auth-enabled
// one, the same two-phase handoff waitMySQLReady's own doc comment
// describes for MySQL.
func waitMongoReady(ctx context.Context, t *testing.T, rt docker.Runtime, containerName string, probe []string, timeout time.Duration) {
	t.Helper()
	waitReady(ctx, t, rt, containerName, probe, timeout)
	time.Sleep(2 * time.Second)
	waitReady(ctx, t, rt, containerName, probe, timeout)
}

// TestContainerRestorer_Restore_MongoDB_Live proves mongoRestoreCmd's
// drop-then-restore fix against a real MongoDB container: seed a
// collection, dump it, then create a collection that did not exist at
// backup time. mongorestore --archive --drop alone only drops collections
// present in the archive, so without the mongosh pre-step this new
// collection would survive restore untouched, exactly the gap a live
// investigation confirmed. This test fails against the old command and
// passes against mongoRestoreCmd's current one.
func TestContainerRestorer_Restore_MongoDB_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-mongodb"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "mongo:7",
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": "leveltest",
			"MONGO_INITDB_ROOT_PASSWORD": "leveltestpass",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	pingProbe := []string{"sh", "-c", `mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "db.runCommand({ping:1})"`}
	waitMongoReady(ctx, t, rt, name, pingProbe, 60*time.Second)

	originalMarker := "levelrail-restore-live-original-mongodb-9d4c"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "db.getSiblingDB('leveltest').restore_probe.insertOne({val: '` + originalMarker + `'})"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineMongoDB, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	// The reproduction from the investigation: a collection created after
	// the backup was taken, absent from the archive entirely, so
	// mongorestore --drop alone has nothing in the archive to trigger a
	// drop for it.
	overwriteMarker := "levelrail-restore-live-overwritten-mongodb-5b8f"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "db.getSiblingDB('leveltest').restore_probe.drop(); db.getSiblingDB('leveltest').unrelated_collection.insertOne({val: '` + overwriteMarker + `'})"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineMongoDB, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify directly against the live database: restore_probe (with the
	// original marker) must be back, and unrelated_collection, created
	// after the backup and never present in the archive, must be gone,
	// proving the mongosh pre-step actually cleared it rather than
	// mongorestore --drop silently leaving it in place.
	check, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "print(JSON.stringify(db.getSiblingDB('leveltest').restore_probe.find().toArray()))"`,
	})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "print(db.getSiblingDB('leveltest').getCollectionNames().join(','))"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if strings.Contains(string(goneOut), "unrelated_collection") {
		t.Errorf("unrelated_collection still exists after restore (output %q), want it gone: it was created after the backup and never present in the archive", goneOut)
	}

	// The root user itself lives in admin.system.users, which the archive
	// also carries (mongodump with no --db captures the whole server); this
	// confirms the mongosh pre-step's exclusion of admin never broke
	// authentication.
	authCheck, err := rt.Exec(ctx, name, pingProbe)
	if err != nil {
		t.Fatalf("Exec() auth-still-works check error = %v", err)
	}
	authOut, err := io.ReadAll(authCheck)
	_ = authCheck.Close()
	if err != nil {
		t.Fatalf("reading auth-still-works check: %v", err)
	}
	if !strings.Contains(string(authOut), "ok: 1") {
		t.Errorf("post-restore ping = %q, want root auth to still work after restore", authOut)
	}
}

// waitMariaDBReady mirrors waitMySQLReady: MariaDB's official image runs
// the same two-phase entrypoint (temporary init server, then the real
// one) MySQL's does, so a single successful probe carries the same
// false-ready risk waitMySQLReady's own doc comment describes.
func waitMariaDBReady(ctx context.Context, t *testing.T, rt docker.Runtime, containerName string, probe []string, timeout time.Duration) {
	t.Helper()
	waitReady(ctx, t, rt, containerName, probe, timeout)
	time.Sleep(2 * time.Second)
	waitReady(ctx, t, rt, containerName, probe, timeout)
}

// TestContainerRestorer_Restore_MariaDB_Live is
// TestContainerRestorer_Restore_MySQL_Live's exact MariaDB counterpart:
// real container, real dump, a real overwriting change, a real restore,
// and a direct query against the live database afterward. MariaDB 11's
// image removes the mysql client binary entirely (see mariadbRestoreCmd's
// own doc comment), so this is the only proof the mariadb client and
// MARIADB_* env vars actually work end to end, not just that the command
// string looks plausible on paper.
func TestContainerRestorer_Restore_MariaDB_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-mariadb"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "mariadb:11",
		Env: map[string]string{
			"MARIADB_ROOT_PASSWORD": "leveltestroot",
			"MARIADB_DATABASE":      "leveltest",
			"MARIADB_USER":          "leveltest",
			"MARIADB_PASSWORD":      "leveltestpass",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitMariaDBReady(ctx, t, rt, name, []string{"sh", "-c", `mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" leveltest -e "SELECT 1"`}, 60*time.Second)

	originalMarker := "levelrail-restore-live-original-mariadb-7c2f"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" leveltest -e "CREATE TABLE restore_probe (val varchar(128)); INSERT INTO restore_probe VALUES ('` + originalMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineMariaDB, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	overwriteMarker := "levelrail-restore-live-overwritten-mariadb-4b8e"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" leveltest -e "DROP TABLE restore_probe; CREATE TABLE unrelated_table (val varchar(128)); INSERT INTO unrelated_table VALUES ('` + overwriteMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineMariaDB, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	check, err := rt.Exec(ctx, name, []string{"sh", "-c", `mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" leveltest -N -e "SELECT val FROM restore_probe;"`})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"sh", "-c", `mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" leveltest -N -e "SHOW TABLES LIKE 'unrelated_table';"`})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if strings.Contains(string(goneOut), "unrelated_table") {
		t.Errorf("unrelated_table still exists after restore (output %q), want the overwrite fully replaced, not merged", goneOut)
	}
}

// waitClickHouseReady mirrors waitMySQLReady: the official
// clickhouse-server image's entrypoint also runs a temporary background
// server to process docker-entrypoint-initdb.d init scripts (triggered
// here by CLICKHOUSE_DB/CLICKHOUSE_USER/CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT
// populating that directory), then kills it and execs the real server in
// its place, so a single successful probe carries the same false-ready
// risk waitMySQLReady's own doc comment describes for MySQL.
func waitClickHouseReady(ctx context.Context, t *testing.T, rt docker.Runtime, containerName string, probe []string, timeout time.Duration) {
	t.Helper()
	waitReady(ctx, t, rt, containerName, probe, timeout)
	time.Sleep(2 * time.Second)
	waitReady(ctx, t, rt, containerName, probe, timeout)
}

// TestContainerRestorer_Restore_ClickHouse_Live proves clickhouseRestoreCmd
// against a real ClickHouse container: real dump, a real overwriting
// change, a real restore, then a direct query against the live database.
// clickhouseDumpCmd's DDL-plus-INSERT script (see its own doc comment) has
// no equivalent in the fake-client unit tests for whether the SQL it
// generates is actually valid to feed back into a real server; this is
// that proof, including that CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT grants
// the non-default user clickhouseRestoreCmd runs as enough privilege to
// drop and recreate the database.
func TestContainerRestorer_Restore_ClickHouse_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-clickhouse"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "clickhouse/clickhouse-server:24.8",
		Env: map[string]string{
			"CLICKHOUSE_DB":                        "leveltest",
			"CLICKHOUSE_USER":                      "leveltest",
			"CLICKHOUSE_PASSWORD":                  "leveltestpass",
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": "1",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// --database "$CLICKHOUSE_DB" here, not a bare SELECT 1: the entrypoint
	// creates that database after the server starts accepting connections,
	// so a probe with no database context can pass before it exists, the
	// same false-ready window postgresDumpCmd's own live test guards
	// against (see waitReady's caller there).
	waitClickHouseReady(ctx, t, rt, name, []string{"sh", "-c", `clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "SELECT 1"`}, 30*time.Second)

	originalMarker := "levelrail-restore-live-original-clickhouse-5e9a"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "CREATE TABLE restore_probe (val String) ENGINE = MergeTree ORDER BY val" && clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "INSERT INTO restore_probe VALUES ('` + originalMarker + `')"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineClickHouse, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	overwriteMarker := "levelrail-restore-live-overwritten-clickhouse-1d6c"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "DROP TABLE restore_probe" && clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "CREATE TABLE unrelated_table (val String) ENGINE = MergeTree ORDER BY val" && clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "INSERT INTO unrelated_table VALUES ('` + overwriteMarker + `')"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineClickHouse, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	check, err := rt.Exec(ctx, name, []string{"sh", "-c", `clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "SELECT val FROM restore_probe"`})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"sh", "-c", `clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "SHOW TABLES"`})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if strings.Contains(string(goneOut), "unrelated_table") {
		t.Errorf("unrelated_table still exists after restore (output %q), want the overwrite fully replaced, not merged", goneOut)
	}
}

// TestContainerRestorer_Restore_KeyDB_Live is
// TestContainerRestorer_Restore_Redis_Live's exact KeyDB counterpart: same
// write-then-stop-then-start RDB reload path, but through keydb-cli
// against the eqalpha/keydb image, proving restoreRedisLike's cliBin
// parameterization actually works against the real binary, not just
// redis-cli under a different name in a fake client's recorded calls.
func TestContainerRestorer_Restore_KeyDB_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-keydb"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "eqalpha/keydb:latest",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitReady(ctx, t, rt, name, []string{"keydb-cli", "PING"}, 15*time.Second)

	originalMarker := "levelrail-restore-live-original-keydb-3a7d"
	seed, err := rt.Exec(ctx, name, []string{"keydb-cli", "SET", "restore_probe", originalMarker})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineKeyDB, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	overwriteMarker := "levelrail-restore-live-overwritten-keydb-8f1b"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		"keydb-cli DEL restore_probe && keydb-cli SET unrelated_key " + overwriteMarker,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineKeyDB, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	waitReady(ctx, t, rt, name, []string{"keydb-cli", "PING"}, 15*time.Second)

	check, err := rt.Exec(ctx, name, []string{"keydb-cli", "GET", "restore_probe"})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"keydb-cli", "EXISTS", "unrelated_key"})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if !strings.Contains(string(goneOut), "0") {
		t.Errorf("unrelated_key EXISTS = %q, want \"0\" (gone), restore should have replaced it with the pre-overwrite snapshot", goneOut)
	}
}

// TestContainerRestorer_Restore_Dragonfly_Live is
// TestContainerRestorer_Restore_Redis_Live's Dragonfly counterpart, with
// the container started the same way internal/reconcile/database's
// Controller starts one: --dbfilename dump as the command override, so
// the RDB restoreRedisLike writes to /data/dump.rdb is the exact file
// Dragonfly reloads on startup (its own default dbfilename embeds a
// timestamp and would never match). Also guards dragonflyDumpCmd's own
// stdout redirect: without it, SAVE's "OK" reply lands ahead of the RDB
// bytes and the restored container fails to come back up at all.
func TestContainerRestorer_Restore_Dragonfly_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-dragonfly"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:    name,
		Image:   "docker.dragonflydb.io/dragonflydb/dragonfly:v1.27.1",
		Command: []string{"--dbfilename", "dump"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitReady(ctx, t, rt, name, []string{"redis-cli", "PING"}, 15*time.Second)

	originalMarker := "levelrail-restore-live-original-dragonfly-2c9f"
	seed, err := rt.Exec(ctx, name, []string{"redis-cli", "SET", "restore_probe", originalMarker})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineDragonfly, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	overwriteMarker := "levelrail-restore-live-overwritten-dragonfly-6e3a"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		"redis-cli DEL restore_probe && redis-cli SET unrelated_key " + overwriteMarker,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineDragonfly, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	waitReady(ctx, t, rt, name, []string{"redis-cli", "PING"}, 15*time.Second)

	check, err := rt.Exec(ctx, name, []string{"redis-cli", "GET", "restore_probe"})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"redis-cli", "EXISTS", "unrelated_key"})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if !strings.Contains(string(goneOut), "0") {
		t.Errorf("unrelated_key EXISTS = %q, want \"0\" (gone), restore should have replaced it with the pre-overwrite snapshot", goneOut)
	}
}
