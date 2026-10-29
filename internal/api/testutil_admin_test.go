package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// bakedAdmin is the state a real admin login leaves behind: the table rows
// it changed plus the session cookie, built once so tests skip the
// ~40 ms password round trip.
type bakedAdmin struct {
	path   string
	tables []string
	cookie http.Cookie
}

var (
	bakedAdminOnce sync.Once
	bakedAdminVal  *bakedAdmin
	bakedAdminErr  error
)

func bakedAdminState(t *testing.T) *bakedAdmin {
	t.Helper()
	bakedAdminOnce.Do(func() { bakedAdminVal, bakedAdminErr = buildBakedAdmin(t) })
	if bakedAdminErr != nil {
		t.Fatalf("build baked admin template: %v", bakedAdminErr)
	}
	return bakedAdminVal
}

func buildBakedAdmin(t *testing.T) (*bakedAdmin, error) {
	plain := migrationTemplateDB(t)
	dir, err := os.MkdirTemp("", "levelrail-test-admin-*")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "admin.db")
	if err := copyFileContents(plain, path); err != nil {
		return nil, err
	}
	ctx := context.Background()
	db, err := store.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	rt := NewRouter(discardLogger(), testBrand(), db)
	cookie, err := loginTestSessionSlow(rt, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	tables, err := changedTables(ctx, db, plain)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}
	return &bakedAdmin{path: path, tables: tables, cookie: *cookie}, nil
}

// changedTables lists tables whose rows differ from the plain template.
func changedTables(ctx context.Context, db *store.DB, plain string) ([]string, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS plain`, plain); err != nil {
		return nil, err
	}
	defer func() { _, _ = conn.ExecContext(ctx, `DETACH DATABASE plain`) }()
	names, err := tableNames(ctx, conn)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range names {
		var diff int
		q := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM (SELECT * FROM main."%[1]s" EXCEPT SELECT * FROM plain."%[1]s"))`, n) //nolint:gosec // names come from sqlite_master, test-only
		if err := conn.QueryRowContext(ctx, q).Scan(&diff); err != nil {
			return nil, fmt.Errorf("diff %s: %w", n, err)
		}
		if diff == 1 {
			out = append(out, n)
		}
	}
	return out, nil
}

func tableNames(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name FROM main.sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// installBakedAdmin copies the admin login rows into a fresh test DB.
func installBakedAdmin(t *testing.T, db *store.DB) *http.Cookie {
	t.Helper()
	b := bakedAdminState(t)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("baked admin conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS baked`, b.path); err != nil {
		t.Fatalf("attach baked admin: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `DETACH DATABASE baked`) }()
	// Rows reference each other across tables; skip FK checks during the copy.
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("baked admin pragma: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`) }()
	for _, n := range b.tables {
		q := fmt.Sprintf(`INSERT OR IGNORE INTO main."%[1]s" SELECT * FROM baked."%[1]s"`, n) //nolint:gosec // names come from sqlite_master, test-only
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("copy baked admin table %s: %v", n, err)
		}
	}
	c := b.cookie
	return &c
}

// loginTestSessionSlow is the real HTTP password login, kept for DBs that
// already hold users and for building the baked template.
func loginTestSessionSlow(rt *Router, db *store.DB) (*http.Cookie, error) {
	if err := BootstrapAdmin(context.Background(), db, testAdminUsername, testAdminPassword); err != nil {
		return nil, err
	}
	body := strings.NewReader(`{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return nil, fmt.Errorf("login setup: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c, nil
		}
	}
	return nil, fmt.Errorf("login setup: no session cookie returned")
}
