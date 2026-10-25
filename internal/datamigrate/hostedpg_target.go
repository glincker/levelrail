package datamigrate

import (
	"bufio"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// TargetState is what the managed target already holds, read before the move.
type TargetState struct {
	// Known is false when the target could not be queried.
	Known bool
	Roles []string
	// UserTables counts tables outside the system schemas: non-zero means an
	// earlier run (or other data) lives there.
	UserTables int
}

// TargetStateCommand prints the target cluster's roles and its user table
// count. It runs inside the managed container, where the superuser variables are set.
func TargetStateCommand() []string {
	return []string{"sh", "-c", `psql --no-password -At -F '|' -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$POSTGRES_USER" <<'SQL'
SELECT 'role|' || rolname FROM pg_roles ORDER BY rolname;
SELECT 'usertables|' || count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog', 'information_schema');
SQL`}
}

// ParseTargetState reads the output of TargetStateCommand.
func ParseTargetState(out string) TargetState {
	st := TargetState{Known: true}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "|")
		switch {
		case !ok:
		case key == "role":
			st.Roles = append(st.Roles, val)
		case key == "usertables":
			st.UserTables, _ = strconv.Atoi(val)
		}
	}
	return st
}

// MissingRoles lists the policy roles the target does not have. Role names are
// case sensitive in Postgres, so the comparison is exact.
func MissingRoles(policyRoles, targetRoles []string) []string {
	have := toSet(targetRoles...)
	var missing []string
	for _, r := range policyRoles {
		if !have[r] {
			missing = append(missing, r)
		}
	}
	sort.Strings(missing)
	return missing
}

var safeRoleRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$.\-]{0,62}$`)

// CreateRolesCommand creates each role NOLOGIN NOINHERIT when absent, so it is
// safe to repeat. Names outside a conservative character set are refused
// rather than quoted, because they come from the source database.
func CreateRolesCommand(roles []string) ([]string, error) {
	var sb strings.Builder
	sb.WriteString(`psql --no-password -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$POSTGRES_USER" <<'SQL'` + "\n")
	for _, r := range roles {
		if !safeRoleRe.MatchString(r) {
			return nil, fmt.Errorf("the role %q has characters that cannot be created automatically", r)
		}
		fmt.Fprintf(&sb, "DO $do$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%[1]s') THEN CREATE ROLE \"%[1]s\" NOLOGIN NOINHERIT; END IF; END $do$;\n", r)
	}
	sb.WriteString("SQL")
	return []string{"sh", "-c", sb.String()}, nil
}

// ObjectCountsCommand counts the non-table objects on the target after the restore.
func ObjectCountsCommand() []string {
	return []string{"sh", "-c", `psql --no-password -At -F '|' -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$POSTGRES_USER" <<'SQL'
SELECT 'sequences|' || count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = 'S' AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname !~ '^pg_toast';
SELECT 'views|' || count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('v', 'm') AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname !~ '^pg_toast';
SELECT 'functions|' || count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE p.prokind IN ('f', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e');
SELECT 'extensions|' || count(*) FROM pg_extension WHERE extname <> 'plpgsql';
SQL`}
}

// ParseObjectCounts reads the output of ObjectCountsCommand.
func ParseObjectCounts(out string) *ObjectCounts {
	oc := &ObjectCounts{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "|")
		n, _ := strconv.Atoi(val)
		switch {
		case !ok:
		case key == "sequences":
			oc.Sequences = n
		case key == "views":
			oc.Views = n
		case key == "functions":
			oc.Functions = n
		case key == "extensions":
			oc.Extensions = n
		}
	}
	return oc
}

// execStderrLines is how much of a failed command's stderr reaches the user.
const execStderrLines = 3

// tidyExecError replaces the docker exec wrapper, which prints the whole
// argv (the probe script), with the exit code and the last stderr lines. The
// full text belongs in the debug log, scrubbed by the caller.
func tidyExecError(err error) error {
	var ee *docker.ExecExitError
	if err == nil || !errors.As(err, &ee) {
		return err
	}
	var lines []string
	for _, l := range strings.Split(ee.Stderr, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > execStderrLines {
		lines = lines[len(lines)-execStderrLines:]
	}
	short := fmt.Sprintf("exited %d: %s", ee.ExitCode, strings.Join(lines, " | "))
	return errors.New(strings.Replace(err.Error(), ee.Error(), short, 1))
}
