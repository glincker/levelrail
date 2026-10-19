// Package dbrotate rotates a managed database's owner password in the
// engine itself, then switches the stored credential every app reads.
package dbrotate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Exec is the slice of the container runtime a driver needs.
type Exec interface {
	ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error)
}

// Driver changes and checks one engine's owner password. Passwords travel on
// stdin only, never in a command line or an error.
type Driver interface {
	// Set makes newPassword the owner password. adminPassword is the one in
	// force now, for engines that authenticate the admin session.
	Set(ctx context.Context, ex Exec, containerID, database, adminPassword, newPassword string) error
	// Verify logs in as the owner with password and fails if it is refused.
	Verify(ctx context.Context, ex Exec, containerID, database, password string) error
}

// ErrEngine wraps every engine-side failure. It never carries engine output.
var ErrEngine = errors.New("database engine refused the statement")

var identRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// DriverFor returns the driver for engine, or false when the engine has no
// owner password this platform can rotate (Redis family, search engines,
// ClickHouse, Memcached).
func DriverFor(engine string) (Driver, bool) {
	switch engine {
	case store.EnginePostgres:
		return postgresDriver{}, true
	case store.EngineMySQL, store.EngineMariaDB:
		return mysqlDriver{}, true
	case store.EngineMongoDB:
		return mongoDriver{}, true
	}
	return nil, false
}

func validate(database string, passwords ...string) error {
	if !identRe.MatchString(database) {
		return fmt.Errorf("dbrotate: database name %q is not a plain identifier", database)
	}
	for _, p := range passwords {
		if p == "" || strings.ContainsAny(p, "\r\n") {
			return errors.New("dbrotate: password is empty or multi-line")
		}
	}
	return nil
}

func run(ctx context.Context, ex Exec, containerID string, cmd []string, stdin string) (string, error) {
	rc, err := ex.ExecWithInput(ctx, containerID, cmd, strings.NewReader(stdin))
	if err != nil {
		var ee *docker.ExecExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%w (exit %d)", ErrEngine, ee.ExitCode)
		}
		return "", fmt.Errorf("dbrotate: exec: %w", err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		var ee *docker.ExecExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%w (exit %d)", ErrEngine, ee.ExitCode)
		}
		return "", fmt.Errorf("dbrotate: read exec output: %w", err)
	}
	return string(out), nil
}

type postgresDriver struct{}

// The pg_hba of the official image trusts loopback, so verification dials the
// container's own address where the password is enforced.
const postgresVerifyScript = `IFS= read -r PGPASSWORD; export PGPASSWORD; H=$(hostname -i 2>/dev/null); H=${H%% *}; [ -n "$H" ] || exit 3; exec psql -X -q --no-password -h "$H" -U "$POSTGRES_USER" -d "$POSTGRES_USER" -tAc "select 1"`

func (postgresDriver) Set(ctx context.Context, ex Exec, containerID, database, _, newPassword string) error {
	if err := validate(database, newPassword); err != nil {
		return err
	}
	pg := dbaccess.Postgres{Exec: ex, ContainerID: containerID, Admin: database, Database: database}
	if err := pg.SetOwnerPassword(ctx, newPassword); err != nil {
		return fmt.Errorf("%w: %s", ErrEngine, errorSummary(err))
	}
	return nil
}

func (postgresDriver) Verify(ctx context.Context, ex Exec, containerID, database, password string) error {
	if err := validate(database, password); err != nil {
		return err
	}
	_, err := run(ctx, ex, containerID, []string{"sh", "-c", postgresVerifyScript}, password+"\n")
	return err
}

// errorSummary keeps a dbaccess statement error's engine ERROR lines, which
// never echo the verifier.
func errorSummary(err error) string {
	var se *dbaccess.StatementError
	if errors.As(err, &se) {
		return se.Message
	}
	return "exec failed"
}

type mysqlDriver struct{}

const mysqlAdminScript = `IFS= read -r MYSQL_PWD; export MYSQL_PWD; CLI=$(command -v mariadb || command -v mysql) || exit 127; exec "$CLI" -uroot --batch --skip-column-names`

func mysqlVerifyScript(user string) string {
	return `IFS= read -r MYSQL_PWD; export MYSQL_PWD; CLI=$(command -v mariadb || command -v mysql) || exit 127; exec "$CLI" -u'` + user + `' --batch --skip-column-names -e 'SELECT 1'`
}

func sqlLiteral(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `''`).Replace(s) + "'"
}

func (mysqlDriver) Set(ctx context.Context, ex Exec, containerID, database, adminPassword, newPassword string) error {
	if err := validate(database, adminPassword, newPassword); err != nil {
		return err
	}
	pw := sqlLiteral(newPassword)
	sql := "ALTER USER IF EXISTS 'root'@'localhost' IDENTIFIED BY " + pw + ";\n" +
		"ALTER USER IF EXISTS 'root'@'%' IDENTIFIED BY " + pw + ";\n" +
		"ALTER USER IF EXISTS " + sqlLiteral(database) + "@'%' IDENTIFIED BY " + pw + ";\n" +
		"FLUSH PRIVILEGES;\n"
	_, err := run(ctx, ex, containerID, []string{"sh", "-c", mysqlAdminScript}, adminPassword+"\n"+sql)
	return err
}

func (mysqlDriver) Verify(ctx context.Context, ex Exec, containerID, database, password string) error {
	if err := validate(database, password); err != nil {
		return err
	}
	_, err := run(ctx, ex, containerID, []string{"sh", "-c", mysqlVerifyScript(database)}, password+"\n")
	return err
}

type mongoDriver struct{}

const (
	mongoShellScript = `IFS= read -r MONGOPW; export MONGOPW; exec mongosh --quiet --norc`
	mongoOK          = "LR_OK"
)

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (mongoDriver) Set(ctx context.Context, ex Exec, containerID, database, adminPassword, newPassword string) error {
	if err := validate(database, adminPassword, newPassword); err != nil {
		return err
	}
	user := jsString(database)
	script := "try { const a = db.getSiblingDB('admin'); a.auth(" + user + ", process.env.MONGOPW); a.changeUserPassword(" + user + ", " + jsString(newPassword) + "); print('" + mongoOK + "'); } catch (e) { print('LR_FAIL'); }\n"
	out, err := run(ctx, ex, containerID, []string{"sh", "-c", mongoShellScript}, adminPassword+"\n"+script)
	if err != nil {
		return err
	}
	if !strings.Contains(out, mongoOK) {
		return fmt.Errorf("%w (mongosh)", ErrEngine)
	}
	return nil
}

func (mongoDriver) Verify(ctx context.Context, ex Exec, containerID, database, password string) error {
	if err := validate(database, password); err != nil {
		return err
	}
	script := "try { db.getSiblingDB('admin').auth(" + jsString(database) + ", process.env.MONGOPW); print('" + mongoOK + "'); } catch (e) { print('LR_FAIL'); }\n"
	out, err := run(ctx, ex, containerID, []string{"sh", "-c", mongoShellScript}, password+"\n"+script)
	if err != nil {
		return err
	}
	if !strings.Contains(out, mongoOK) {
		return fmt.Errorf("%w (mongosh login)", ErrEngine)
	}
	return nil
}
