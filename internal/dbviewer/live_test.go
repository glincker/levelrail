package dbviewer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// cliExecer runs commands through `docker exec -i`, mirroring what the
// agent's Exec API does, so the live tests exercise the real scripts.
type cliExecer struct{}

func (cliExecer) ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	args := append([]string{"exec", "-i", containerID}, cmd...)
	c := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // test helper, container name comes from a test env var
	c.Stdin = stdin
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	rc := &errAfter{Reader: &stdout}
	if err != nil {
		code := 1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		rc.err = &docker.ExecExitError{Cmd: cmd, Container: containerID, ExitCode: code, Stderr: stderr.String()}
	}
	return rc, nil
}

type errAfter struct {
	io.Reader
	err error
}

func (e *errAfter) Read(p []byte) (int, error) {
	n, err := e.Reader.Read(p)
	if err == io.EOF && e.err != nil {
		return n, e.err
	}
	return n, err
}

func (e *errAfter) Close() error { return nil }

func liveTarget(t *testing.T, envKey string, d Dialect) Target {
	t.Helper()
	if testing.Short() {
		t.Skip("live test")
	}
	name := os.Getenv(envKey)
	if name == "" {
		t.Skipf("%s not set", envKey)
	}
	return Target{Exec: cliExecer{}, ContainerID: name, Dialect: d, Limits: Limits{
		Timeout: 5 * time.Second, MaxRows: 50, MaxBytes: 1 << 20, MaxCellBytes: 1 << 10, SchemaRows: 1000,
	}}
}

func TestLivePostgres(t *testing.T) {
	tg := liveTarget(t, "APP_LIVE_PG_CONTAINER", DialectPostgres)
	ctx := context.Background()

	setup := []string{
		"DROP TABLE IF EXISTS lr_items",
		"CREATE TABLE lr_items (id serial PRIMARY KEY, name text, note text)",
		"INSERT INTO lr_items (name, note) SELECT 'item ' || g, CASE WHEN g % 2 = 0 THEN NULL ELSE '' END FROM generate_series(1, 120) g",
		"ANALYZE lr_items",
	}
	for _, s := range setup {
		if _, err := tg.Run(ctx, Statement{SQL: s, Kind: KindWrite}, true); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}

	t.Run("select caps rows and keeps null vs empty", func(t *testing.T) {
		st, _ := Check(DialectPostgres, "select id, name, note from lr_items order by id", false)
		res, err := tg.Run(ctx, st, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.RowCount != 50 || !res.Truncated {
			t.Fatalf("rows=%d truncated=%v", res.RowCount, res.Truncated)
		}
		if res.Rows[0][2] == nil || *res.Rows[0][2] != "" {
			t.Errorf("row 1 note should be empty string, got %v", res.Rows[0][2])
		}
		if res.Rows[1][2] != nil {
			t.Errorf("row 2 note should be NULL")
		}
	})

	t.Run("write is blocked in read-only mode by the server", func(t *testing.T) {
		st := Statement{SQL: "delete from lr_items", Kind: KindShow}
		_, err := tg.Run(ctx, st, false)
		var qe *QueryError
		if err == nil || !errors.As(err, &qe) || !strings.Contains(qe.Message, "read-only") {
			t.Fatalf("err = %v, want read-only transaction error", err)
		}
	})

	t.Run("commit smuggling cannot leave read-only", func(t *testing.T) {
		st := Statement{SQL: "select 1; commit; delete from lr_items", Kind: KindQuery}
		_, _ = tg.Run(ctx, st, false)
		res, err := tg.RunTrusted(ctx, "select count(*) from lr_items")
		if err != nil || *res.Rows[0][0] != "120" {
			t.Fatalf("rows survived? res=%+v err=%v", res, err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		st, _ := Check(DialectPostgres, "select pg_sleep(30)", false)
		_, err := tg.Run(ctx, st, false)
		if err == nil || !strings.Contains(err.Error(), "statement timeout") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("schema", func(t *testing.T) {
		nodes, err := tg.Schema(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var found *Table
		for i := range nodes {
			for j := range nodes[i].Tables {
				if nodes[i].Name == "public" && nodes[i].Tables[j].Name == "lr_items" {
					found = &nodes[i].Tables[j]
				}
			}
		}
		if found == nil || len(found.Columns) != 3 || !found.Columns[0].PrimaryKey || len(found.Indexes) != 1 || found.RowEstimate != 120 {
			t.Fatalf("lr_items = %+v", found)
		}
	})

	t.Run("table page sort and filter", func(t *testing.T) {
		p, err := tg.TablePage(ctx, PageQuery{Schema: "public", Table: "lr_items", Limit: 10, SortColumn: "id", SortDesc: true, FilterColumn: "name", FilterOp: OpContains, FilterValue: "item 1"})
		if err != nil {
			t.Fatal(err)
		}
		if p.RowCount != 10 || !p.HasMore || *p.Rows[0][0] != "120" {
			t.Fatalf("page = %+v first=%v", p, p.Rows[0][0])
		}
		if _, err := tg.TablePage(ctx, PageQuery{Schema: "public", Table: "lr_items", SortColumn: "id; drop table lr_items"}); err == nil {
			t.Fatal("injected sort column must be rejected")
		}
	})

	t.Run("explain", func(t *testing.T) {
		st, _ := Check(DialectPostgres, "select * from lr_items where id = 3", false)
		res, err := tg.Explain(ctx, st, true)
		if err != nil || res.RowCount == 0 || !strings.Contains(*res.Rows[0][0], "lr_items") {
			t.Fatalf("explain res=%+v err=%v", res, err)
		}
	})

	t.Run("write mode commits", func(t *testing.T) {
		st, _ := Check(DialectPostgres, "update lr_items set note = 'x' where id = 1 returning id", true)
		res, err := tg.Run(ctx, st, true)
		if err != nil || res.RowCount != 1 {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})
}

func TestLiveRedis(t *testing.T) {
	tg := liveTarget(t, "APP_LIVE_REDIS_CONTAINER", DialectPostgres)
	ctx := context.Background()
	for _, args := range [][]string{{"SET", "lr:a", "hello world"}, {"RPUSH", "lr:l", "x", "y"}, {"HSET", "lr:h", "f", "v"}} {
		rc, _ := cliExecer{}.ExecWithInput(ctx, tg.ContainerID, append([]string{"redis-cli"}, args...), strings.NewReader(""))
		_, _ = io.Copy(io.Discard, rc)
	}
	scan, err := tg.ScanKeys(ctx, "0", "lr:*", 100)
	if err != nil || len(scan.Keys) != 3 {
		t.Fatalf("scan=%+v err=%v", scan, err)
	}
	v, err := tg.GetKey(ctx, "lr:a")
	if err != nil || v.Type != "string" || len(v.Lines) != 1 || v.Lines[0] != "hello world" {
		t.Fatalf("get=%+v err=%v", v, err)
	}
	h, err := tg.GetKey(ctx, "lr:h")
	if err != nil || h.Type != "hash" || len(h.Lines) != 2 {
		t.Fatalf("hash=%+v err=%v", h, err)
	}
	if _, err := tg.GetKey(ctx, "lr:missing"); err == nil {
		t.Fatal("missing key should be ErrNotFound")
	}
}

func TestLiveMySQL(t *testing.T) {
	tg := liveTarget(t, "APP_LIVE_MYSQL_CONTAINER", DialectMySQL)
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS lr_items",
		"CREATE TABLE lr_items (id INT PRIMARY KEY AUTO_INCREMENT, name VARCHAR(50), note TEXT, KEY idx_name (name))",
		"INSERT INTO lr_items (name, note) VALUES ('a', NULL), ('b', ''), ('c', 'x<y>&z')",
	} {
		if _, err := tg.Run(ctx, Statement{SQL: s, Kind: KindWrite}, true); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}

	st, _ := Check(DialectMySQL, "select id, name, note from lr_items order by id", false)
	res, err := tg.Run(ctx, st, false)
	if err != nil || res.RowCount != 3 || len(res.Columns) != 3 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.Rows[0][2] != nil || res.Rows[1][2] == nil || *res.Rows[1][2] != "" || *res.Rows[2][2] != "x<y>&z" {
		t.Errorf("null/empty/escape handling wrong: %v %v %v", res.Rows[0][2], res.Rows[1][2], res.Rows[2][2])
	}

	if _, err := tg.Run(ctx, Statement{SQL: "delete from lr_items", Kind: KindShow}, false); err == nil {
		t.Fatal("write inside read-only transaction must fail")
	}

	nodes, err := tg.Schema(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found *Table
	for i := range nodes {
		for j := range nodes[i].Tables {
			if nodes[i].Tables[j].Name == "lr_items" {
				found = &nodes[i].Tables[j]
			}
		}
	}
	if found == nil || len(found.Columns) != 3 || !found.Columns[0].PrimaryKey || len(found.Indexes) != 2 {
		t.Fatalf("lr_items = %+v", found)
	}

	p, err := tg.TablePage(ctx, PageQuery{Schema: "app", Table: "lr_items", Limit: 2, SortColumn: "id", SortDesc: true})
	if err != nil || p.RowCount != 2 || !p.HasMore || *p.Rows[0][0] != "3" {
		t.Fatalf("page=%+v err=%v", p, err)
	}
	ex, _ := Check(DialectMySQL, "select * from lr_items where id = 1", false)
	if r, err := tg.Explain(ctx, ex, false); err != nil || r.RowCount == 0 {
		t.Fatalf("explain r=%+v err=%v", r, err)
	}
}
