package dbaccess

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// dockerCLIExec adapts the docker CLI to Exec so the real SQL runs against a
// real Postgres. Exit failures carry stderr the way the engine client does.
type dockerCLIExec struct{}

func (dockerCLIExec) ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	args := append([]string{"exec", "-i", containerID}, cmd...)
	c := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // test helper running the docker CLI on fixed arguments
	c.Stdin = stdin
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	if err := c.Run(); err != nil {
		code := -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return io.NopCloser(&errReader{err: &docker.ExecExitError{ExitCode: code, Stderr: stderr.String()}}), nil
	}
	return io.NopCloser(&stdout), nil
}

type errReader struct{ err error }

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }

func dockerOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput() //nolint:gosec // test helper running the docker CLI on fixed arguments
	if err != nil {
		t.Fatalf("docker %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestLivePostgresRoleLifecycle runs every statement this package builds
// against a throwaway postgres container, then proves the privileges with
// real psql sessions authenticated by the generated SCRAM verifier.
func TestLivePostgresRoleLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	const name = "dbaccess-live-pg"
	_ = exec.Command("docker", "rm", "-f", name).Run()
	dockerOut(t, "run", "-d", "--name", name, "-e", "POSTGRES_USER=maindb", "-e", "POSTGRES_PASSWORD=adminpw", "postgres:16-alpine")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })

	ctx := context.Background()
	pg := Postgres{Exec: dockerCLIExec{}, ContainerID: name, Admin: "maindb", Database: "maindb"}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := pg.ListRoles(ctx); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("postgres did not become ready")
		}
		time.Sleep(time.Second)
	}
	if _, err := pg.run(ctx, "CREATE TABLE items (id serial PRIMARY KEY, v text);\nINSERT INTO items (v) VALUES ('a');\n"); err != nil {
		t.Fatal(err)
	}

	psqlAs := func(user, password, sql string) (string, error) {
		// The container's own address, not loopback: loopback is trust in the image's pg_hba.
		args := []string{"exec", "-e", "PGPASSWORD=" + password, "-e", "U=" + user, "-e", "Q=" + sql, name,
			"sh", "-c", `exec psql -h "$(hostname -i)" -U "$U" -d maindb -X -q -t -A -c "$Q"`}
		c := exec.Command("docker", args...) //nolint:gosec // test helper running the docker CLI on arguments built in this test
		out, err := c.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	ro, err := pg.Create(ctx, CreateParamsIn{Role: "report_ro", Preset: PresetReadOnly, ConnLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := psqlAs("report_ro", ro.Password, "SELECT count(*) FROM items"); err != nil || out != "1" {
		t.Fatalf("read-only select = %q, %v", out, err)
	}
	if out, err := psqlAs("report_ro", ro.Password, "INSERT INTO items (v) VALUES ('x')"); err == nil {
		t.Fatalf("read-only role could insert: %s", out)
	}
	if _, err := pg.Create(ctx, CreateParamsIn{Role: "report_ro", Preset: PresetReadOnly}); err != ErrRoleExists {
		t.Fatalf("duplicate create err = %v", err)
	}

	rw, err := pg.Create(ctx, CreateParamsIn{Role: "app_rw", Preset: PresetReadWrite})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := psqlAs("app_rw", rw.Password, "INSERT INTO items (v) VALUES ('y'); SELECT 1"); err != nil {
		t.Fatalf("read-write insert failed: %v %s", err, out)
	}
	if out, err := psqlAs("app_rw", rw.Password, "CREATE TABLE nope (id int)"); err == nil {
		t.Fatalf("read-write role could create a table: %s", out)
	}
	// Default privileges: a table the admin creates later is readable.
	if _, err := pg.run(ctx, "CREATE TABLE later (id int);\nINSERT INTO later VALUES (1);\n"); err != nil {
		t.Fatal(err)
	}
	if out, err := psqlAs("report_ro", ro.Password, "SELECT count(*) FROM later"); err != nil || out != "1" {
		t.Fatalf("default privileges did not cover a later table: %q %v", out, err)
	}

	roles, err := pg.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, r := range roles {
		if r.Name == "report_ro" {
			seen = true
			if r.ConnLimit != 5 || !r.CanLogin {
				t.Errorf("role attrs = %+v", r)
			}
		}
	}
	if !seen {
		t.Fatalf("report_ro missing from %+v", roles)
	}

	rotated, err := pg.Rotate(ctx, "report_ro")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := psqlAs("report_ro", ro.Password, "SELECT 1"); err == nil {
		t.Fatal("old password still works after rotate")
	}
	if out, err := psqlAs("report_ro", rotated.Password, "SELECT 1"); err != nil || out != "1" {
		t.Fatalf("new password failed: %q %v", out, err)
	}

	if err := pg.SetLogin(ctx, "report_ro", false); err != nil {
		t.Fatal(err)
	}
	if _, err := psqlAs("report_ro", rotated.Password, "SELECT 1"); err == nil {
		t.Fatal("disabled role could log in")
	}
	if err := pg.SetLogin(ctx, "maindb", false); err == nil {
		t.Fatal("platform admin role must not be disableable")
	}
	if err := pg.Drop(ctx, "maindb"); err == nil {
		t.Fatal("platform admin role must not be droppable")
	}

	temp, err := pg.Create(ctx, CreateParamsIn{Role: "tmp_live01", Preset: PresetReadOnly, ConnLimit: TempConnLimit, ValidUntil: time.Now().Add(90 * time.Second).UTC().Truncate(time.Second), Temporary: true})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := psqlAs("tmp_live01", temp.Password, "SELECT count(*) FROM items"); err != nil || out == "" {
		t.Fatalf("temp select = %q %v", out, err)
	}
	if err := pg.DropTemp(ctx, "tmp_live01"); err != nil {
		t.Fatal(err)
	}
	if err := pg.DropTemp(ctx, "tmp_live01"); err != nil {
		t.Fatalf("second drop must be idempotent: %v", err)
	}
	if _, err := psqlAs("tmp_live01", temp.Password, "SELECT 1"); err == nil {
		t.Fatal("dropped temp role could log in")
	}
	for _, name := range []string{"report_ro", "app_rw"} {
		if err := pg.Drop(ctx, name); err != nil {
			t.Fatalf("drop %s: %v", name, err)
		}
	}
	after, _ := pg.ListRoles(ctx)
	for _, r := range after {
		if r.Name == "report_ro" || r.Name == "app_rw" || strings.HasPrefix(r.Name, TempRolePrefix) {
			t.Errorf("role %s survived drop", r.Name)
		}
	}

	state, err := pg.TLSState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("tls state before:", state)
	if err := pg.SetRequireTLS(ctx, true); err != nil {
		t.Fatal(err)
	}
	if s, _ := pg.TLSState(ctx); s != TLSRequired {
		t.Fatalf("tls state after require = %q", s)
	}
	if err := pg.SetRequireTLS(ctx, false); err != nil {
		t.Fatal(err)
	}
	if s, _ := pg.TLSState(ctx); s != TLSOptional {
		t.Fatalf("tls state after relax = %q", s)
	}
}
