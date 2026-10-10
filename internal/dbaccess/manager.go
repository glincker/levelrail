package dbaccess

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Exec is the slice of the container runtime this package needs.
type Exec interface {
	ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error)
}

// Sentinel errors the API maps to status codes.
var (
	ErrRoleExists   = errors.New("role already exists")
	ErrRoleNotFound = errors.New("role not found")
	ErrProtected    = errors.New("role is protected")
	ErrTTL          = errors.New("invalid ttl")
)

// StatementError is a database-side failure with only the engine's own
// ERROR lines kept, so no statement text is echoed back.
type StatementError struct{ Message string }

func (e *StatementError) Error() string { return e.Message }

// psqlCommand execs psql over the container's local socket as its admin
// role, the same way the database console does. Writes are enabled: the
// statements here are DDL built by this package.
var psqlCommand = []string{"sh", "-c", `exec psql -X -q --no-password --csv -t -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$POSTGRES_USER"`}

// Postgres manages roles in one managed Postgres container. The platform's
// admin role and the database share a name, which is what Admin holds.
type Postgres struct {
	Exec        Exec
	ContainerID string
	Admin       string
	Database    string
}

func (p Postgres) run(ctx context.Context, script string) (string, error) {
	rc, err := p.Exec.ExecWithInput(ctx, p.ContainerID, psqlCommand, strings.NewReader(script))
	if err != nil {
		return "", wrapExec(err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		return "", wrapExec(err)
	}
	return string(out), nil
}

func wrapExec(err error) error {
	var ee *docker.ExecExitError
	if errors.As(err, &ee) {
		return &StatementError{Message: errorText(ee.Stderr, ee.ExitCode)}
	}
	return fmt.Errorf("dbaccess: exec: %w", err)
}

func errorText(stderr string, exit int) string {
	var keep []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "ERROR:"); i >= 0 {
			keep = append(keep, line[i:])
		} else if strings.HasPrefix(line, "FATAL") {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return fmt.Sprintf("statement failed (exit %d)", exit)
	}
	msg := strings.Join(keep, "\n")
	if len(msg) > 400 {
		msg = msg[:400]
	}
	return msg
}

// ListRoles returns every non-system role with its attributes.
func (p Postgres) ListRoles(ctx context.Context) ([]Role, error) {
	out, err := p.run(ctx, ListRolesSQL+"\n")
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	rows, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("list roles: parse: %w", err)
	}
	return ParseRoles(rows)
}

func (p Postgres) find(ctx context.Context, name string) (Role, error) {
	roles, err := p.ListRoles(ctx)
	if err != nil {
		return Role{}, err
	}
	for _, r := range roles {
		if r.Name == name {
			return r, nil
		}
	}
	return Role{}, ErrRoleNotFound
}

// Credential is a freshly minted login. Password is shown once by the caller
// and never stored.
type Credential struct {
	Role       string
	Password   string
	ValidUntil time.Time
}

// CreateParamsIn is what a caller chooses; the rest is derived.
type CreateParamsIn struct {
	Role       string
	Preset     Preset
	ConnLimit  int
	ValidUntil time.Time
	// Temporary skips the user-name reservation so a generated tmp_ name is accepted.
	Temporary bool
}

// Create makes a login role with the preset's privileges.
func (p Postgres) Create(ctx context.Context, in CreateParamsIn) (Credential, error) {
	if in.Temporary {
		if !strings.HasPrefix(in.Role, TempRolePrefix) {
			return Credential{}, fmt.Errorf("%w: temporary roles must start with %s", ErrRoleName, TempRolePrefix)
		}
	} else if err := ValidateRoleName(in.Role, p.Admin); err != nil {
		return Credential{}, err
	}
	if _, err := p.find(ctx, in.Role); err == nil {
		return Credential{}, ErrRoleExists
	} else if !errors.Is(err, ErrRoleNotFound) {
		return Credential{}, err
	}
	password, err := GeneratePassword()
	if err != nil {
		return Credential{}, err
	}
	verifier, err := ScramVerifier(password)
	if err != nil {
		return Credential{}, err
	}
	script, err := CreateRoleSQL(CreateParams{
		Role: in.Role, Database: p.Database, AdminRole: p.Admin, Preset: in.Preset,
		Verifier: verifier, ConnLimit: in.ConnLimit, ValidUntil: in.ValidUntil,
	})
	if err != nil {
		return Credential{}, err
	}
	if _, err := p.run(ctx, script); err != nil {
		return Credential{}, fmt.Errorf("create role %q: %w", in.Role, err)
	}
	return Credential{Role: in.Role, Password: password, ValidUntil: in.ValidUntil}, nil
}

func (p Postgres) mutable(ctx context.Context, name string) error {
	r, err := p.find(ctx, name)
	if err != nil {
		return err
	}
	if Protected(r, p.Admin) {
		return fmt.Errorf("%w: %q is a platform or system role", ErrProtected, name)
	}
	return nil
}

// Rotate sets a new random password and ends the role's sessions.
func (p Postgres) Rotate(ctx context.Context, name string) (Credential, error) {
	if err := p.mutable(ctx, name); err != nil {
		return Credential{}, err
	}
	password, err := GeneratePassword()
	if err != nil {
		return Credential{}, err
	}
	verifier, err := ScramVerifier(password)
	if err != nil {
		return Credential{}, err
	}
	script, err := RotateSQL(name, verifier)
	if err != nil {
		return Credential{}, err
	}
	if _, err := p.run(ctx, script); err != nil {
		return Credential{}, fmt.Errorf("rotate role %q: %w", name, err)
	}
	return Credential{Role: name, Password: password}, nil
}

// SetLogin enables or disables a role's ability to log in.
func (p Postgres) SetLogin(ctx context.Context, name string, login bool) error {
	if err := p.mutable(ctx, name); err != nil {
		return err
	}
	script, err := SetLoginSQL(name, login)
	if err != nil {
		return err
	}
	if _, err := p.run(ctx, script); err != nil {
		return fmt.Errorf("set login for role %q: %w", name, err)
	}
	return nil
}

// Drop removes a role. A role that is already gone is not an error.
func (p Postgres) Drop(ctx context.Context, name string) error {
	if err := p.mutable(ctx, name); err != nil {
		if errors.Is(err, ErrRoleNotFound) {
			return nil
		}
		return err
	}
	return p.dropUnchecked(ctx, name)
}

// DropTemp removes a temporary role without the existence listing, so the
// sweeper stays idempotent and cheap. Only tmp_ names are accepted.
func (p Postgres) DropTemp(ctx context.Context, name string) error {
	if !strings.HasPrefix(name, TempRolePrefix) {
		return fmt.Errorf("%w: %q is not a temporary role", ErrProtected, name)
	}
	return p.dropUnchecked(ctx, name)
}

func (p Postgres) dropUnchecked(ctx context.Context, name string) error {
	script, err := DropRoleSQL(name, p.Admin)
	if err != nil {
		return err
	}
	if _, err := p.run(ctx, script); err != nil {
		return fmt.Errorf("drop role %q: %w", name, err)
	}
	return nil
}
