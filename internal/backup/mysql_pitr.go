package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// mysqlBinlogPath duplicates internal/reconcile/database.MySQLBinlogPath, the
// same duplicate-a-small-stable-format tradeoff postgresWALArchivePath makes.
const mysqlBinlogPath = "/var/lib/mysql-binlog"

var (
	binlogNameRe   = regexp.MustCompile(`^mysql-bin\.[0-9]{6}$`)
	binlogCoordRe  = regexp.MustCompile(`^(mysql-bin\.[0-9]{6}):([0-9]+)$`)
	dumpPositionRe = regexp.MustCompile(`(?:SOURCE|MASTER)_LOG_FILE='([^']+)',\s*(?:SOURCE|MASTER)_LOG_POS=([0-9]+);`)
)

// BinlogPrefix is the key prefix under which a database's shipped binlog files
// live in its backup target.
func BinlogPrefix(databaseName string) string { return databaseName + "/binlog/" }

// IsBinlogCoordinate reports whether a base backup's LSN column holds a MySQL
// family "file:position" rather than a Postgres LSN.
func IsBinlogCoordinate(lsn string) bool { return binlogCoordRe.MatchString(lsn) }

// The two images name the client binaries and the root password variable differently.
const mysqlShellPrelude = `P="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; DB="${MYSQL_DATABASE:-$MARIADB_DATABASE}"; ` +
	`C=$(command -v mariadb || command -v mysql); D=$(command -v mariadb-dump || command -v mysqldump); ` +
	`B=$(command -v mariadb-binlog || command -v mysqlbinlog); `

func mysqlShell(script string) []string {
	return []string{"bash", "-c", "set -o pipefail; " + mysqlShellPrelude + script}
}

// MySQL 8.4 dropped --master-data, MariaDB still spells it that way.
var mysqlBaseBackupCmd = mysqlShell(
	`if "$D" --help 2>/dev/null | grep -q -- --source-data; then F=--source-data=2; else F=--master-data=2; fi; ` +
		`exec "$D" -uroot -p"$P" --single-transaction "$F" --routines --events --triggers "$DB"`)

// MySQLBaseBackuper is the BaseBackuper for MySQL and MariaDB: a consistent
// logical dump that records the binlog position replay must start from.
type MySQLBaseBackuper struct {
	Runtime Runtime
}

// BaseBackup implements BaseBackuper. The returned reader also reports the
// binlog position through Position once the dump header has been read.
func (b *MySQLBaseBackuper) BaseBackup(ctx context.Context, containerName string) (io.ReadCloser, error) {
	rc, err := b.Runtime.Exec(ctx, containerName, mysqlBaseBackupCmd)
	if err != nil {
		return nil, fmt.Errorf("backup: mysql base backup container %q: %w", containerName, err)
	}
	return &positionReader{ReadCloser: rc}, nil
}

const positionScanLimit = 64 << 10

type positionReader struct {
	io.ReadCloser
	head []byte
	pos  string
}

func (p *positionReader) Read(b []byte) (int, error) {
	n, err := p.ReadCloser.Read(b)
	if p.pos == "" && len(p.head) < positionScanLimit {
		p.head = append(p.head, b[:n]...)
		if m := dumpPositionRe.FindSubmatch(p.head); m != nil {
			p.pos = string(m[1]) + ":" + string(m[2])
			p.head = nil
		}
	}
	return n, err
}

// Position returns "file:offset", or "" when the dump carried no coordinates.
func (p *positionReader) Position() string { return p.pos }

// MySQLWindowEnd returns the latest instant a point-in-time restore can target.
// The clock is read before the rotation, so every event stamped at or before it
// sits in a closed binlog file by the time this returns.
func MySQLWindowEnd(ctx context.Context, rt Runtime, containerName string) (time.Time, error) {
	out, err := execOutput(ctx, rt, containerName, mysqlShell(
		`T=$("$C" -uroot -p"$P" -Nse "SELECT UNIX_TIMESTAMP(NOW())") && `+
			`"$C" -uroot -p"$P" -e "FLUSH BINARY LOGS" && echo "$T"`))
	if err != nil {
		return time.Time{}, fmt.Errorf("backup: mysql window end for %q: %w", containerName, err)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return time.Time{}, fmt.Errorf("backup: mysql window end for %q: empty output", containerName)
	}
	secs, err := strconv.ParseInt(fields[len(fields)-1], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("backup: mysql window end for %q: unexpected output %q", containerName, out)
	}
	return time.Unix(secs, 0).UTC(), nil
}

type binlogFile struct {
	name string
	size int64
}

// listLocalBinlogs returns the closed binlog files (everything but the one
// mysqld is still writing) in order.
func listLocalBinlogs(ctx context.Context, rt Runtime, containerName string) ([]binlogFile, error) {
	script := `cd ` + mysqlBinlogPath + ` && active=$(tail -n 1 mysql-bin.index | sed 's#.*/##'); ` +
		`for f in mysql-bin.[0-9][0-9][0-9][0-9][0-9][0-9]; do [ -f "$f" ] && [ "$f" != "$active" ] && echo "$f $(wc -c < "$f")"; done; exit 0`
	out, err := execOutput(ctx, rt, containerName, []string{"bash", "-c", script})
	if err != nil {
		return nil, fmt.Errorf("list local binlogs: %w", err)
	}
	var files []binlogFile
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || !binlogNameRe.MatchString(f[0]) {
			continue
		}
		n, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		files = append(files, binlogFile{name: f[0], size: n})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

// BinlogShipper copies a database's closed binlog files into its backup target,
// the MySQL family counterpart of WALShipper.
type BinlogShipper struct {
	Runtime  Runtime
	Uploader Uploader
	Lister   Lister
	Logger   *slog.Logger

	mu         sync.Mutex
	idleActive map[string]int64
}

// Ship rotates the active binlog when it has grown since the last rotation, so
// recent writes are in a closed file, then uploads every closed file the target
// does not hold at the same size. It returns how many files it uploaded.
func (s *BinlogShipper) Ship(ctx context.Context, dest Destination, databaseName, containerName string) (int, error) {
	if err := s.rotateIfDirty(ctx, containerName); err != nil {
		return 0, err
	}
	local, err := listLocalBinlogs(ctx, s.Runtime, containerName)
	if err != nil {
		return 0, err
	}
	remoteObjs, err := s.Lister.List(ctx, dest, BinlogPrefix(databaseName))
	if err != nil {
		return 0, fmt.Errorf("list shipped binlogs for %q: %w", databaseName, err)
	}
	remote := make(map[string]int64, len(remoteObjs))
	for _, o := range remoteObjs {
		remote[strings.TrimPrefix(o.Key, BinlogPrefix(databaseName))] = o.Size
	}
	shipped := 0
	for _, f := range local {
		if remote[f.name] == f.size {
			continue
		}
		if err := s.shipOne(ctx, dest, databaseName, containerName, f); err != nil {
			return shipped, err
		}
		shipped++
	}
	return shipped, nil
}

func (s *BinlogShipper) shipOne(ctx context.Context, dest Destination, databaseName, containerName string, f binlogFile) error {
	rc, err := s.Runtime.Exec(ctx, containerName, []string{"cat", mysqlBinlogPath + "/" + f.name})
	if err != nil {
		return fmt.Errorf("read binlog %q: %w", f.name, err)
	}
	defer func() { _ = rc.Close() }()
	if err := s.Uploader.Upload(ctx, dest, BinlogPrefix(databaseName)+f.name, rc, f.size); err != nil {
		return fmt.Errorf("ship binlog %q of %q: %w", f.name, databaseName, err)
	}
	return nil
}

func (s *BinlogShipper) rotateIfDirty(ctx context.Context, containerName string) error {
	size, err := activeBinlogSize(ctx, s.Runtime, containerName)
	if err != nil {
		return err
	}
	s.mu.Lock()
	idle, known := s.idleActive[containerName]
	s.mu.Unlock()
	if known && idle == size {
		return nil
	}
	if _, err := execOutput(ctx, s.Runtime, containerName, mysqlShell(`"$C" -uroot -p"$P" -e "FLUSH BINARY LOGS"`)); err != nil {
		return fmt.Errorf("flush binary logs on %q: %w", containerName, err)
	}
	after, err := activeBinlogSize(ctx, s.Runtime, containerName)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.idleActive == nil {
		s.idleActive = map[string]int64{}
	}
	s.idleActive[containerName] = after
	s.mu.Unlock()
	return nil
}

func activeBinlogSize(ctx context.Context, rt Runtime, containerName string) (int64, error) {
	out, err := execOutput(ctx, rt, containerName, []string{"bash", "-c",
		`cd ` + mysqlBinlogPath + ` && wc -c < "$(tail -n 1 mysql-bin.index | sed 's#.*/##')"`})
	if err != nil {
		return 0, fmt.Errorf("size of active binlog on %q: %w", containerName, err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size of active binlog on %q: unexpected output %q", containerName, out)
	}
	return n, nil
}

// PruneBinlogArchive removes closed local binlog files older than the file the
// oldest kept base backup starts from.
func PruneBinlogArchive(ctx context.Context, rt Runtime, containerName, coordinate string) error {
	if rt == nil {
		return nil
	}
	m := binlogCoordRe.FindStringSubmatch(coordinate)
	if m == nil {
		return fmt.Errorf("refusing to prune with malformed binlog coordinate %q", coordinate)
	}
	script := `cd ` + mysqlBinlogPath + ` && active=$(tail -n 1 mysql-bin.index | sed 's#.*/##'); ` +
		`for f in mysql-bin.[0-9][0-9][0-9][0-9][0-9][0-9]; do [ -f "$f" ] && [ "$f" != "$active" ] && [ "$f" "<" "` + m[1] + `" ] && rm -f "$f"; done; exit 0`
	if _, err := execOutput(ctx, rt, containerName, []string{"bash", "-c", script}); err != nil {
		return fmt.Errorf("exec binlog prune: %w", err)
	}
	return nil
}

// PruneRemoteBinlogs deletes shipped files older than the one coordinate names.
func PruneRemoteBinlogs(ctx context.Context, dest Destination, lister Lister, deleter Deleter, databaseName, coordinate string) (int, error) {
	m := binlogCoordRe.FindStringSubmatch(coordinate)
	if m == nil {
		return 0, fmt.Errorf("malformed binlog coordinate %q", coordinate)
	}
	objs, err := lister.List(ctx, dest, BinlogPrefix(databaseName))
	if err != nil {
		return 0, fmt.Errorf("list shipped binlogs for %q: %w", databaseName, err)
	}
	removed := 0
	for _, o := range objs {
		name := strings.TrimPrefix(o.Key, BinlogPrefix(databaseName))
		if !binlogNameRe.MatchString(name) || name >= m[1] {
			continue
		}
		if err := deleter.Delete(ctx, dest, o.Key); err != nil {
			return removed, fmt.Errorf("delete shipped binlog %q: %w", o.Key, err)
		}
		removed++
	}
	return removed, nil
}

// binlogToolCmd prints the path of the binlog reader an image ships, failing when it has none.
var binlogToolCmd = []string{"bash", "-c", `command -v mariadb-binlog || command -v mysqlbinlog`}

// binlogSQLCmd turns the staged binlog files into SQL for the schema dstDB, stopping
// before stopAt. The schema is renamed because the new resource's own database
// carries a different name than the source's.
func binlogSQLCmd(files []string, startPos int64, stopAt time.Time, srcDB, dstDB string) []string {
	// MySQL's mysqlbinlog applies --database after the rewrite, MariaDB's before it.
	return []string{"bash", "-c", fmt.Sprintf(
		`B=$(command -v mariadb-binlog || command -v mysqlbinlog); case "$B" in *mariadb-binlog) F=%q;; *) F=%q;; esac; `+
			`cd /tmp/pitr-binlog && TZ=UTC exec "$B" --start-position=%d --stop-datetime=%q --database="$F" --rewrite-db=%q %s`,
		srcDB, dstDB, startPos, stopAt.UTC().Format("2006-01-02 15:04:05"), srcDB+"->"+dstDB, strings.Join(files, " "))}
}

// mysqlLoadCmd feeds SQL on stdin into the managed schema.
var mysqlLoadCmd = mysqlShell(`exec "$C" -uroot -p"$P" "$DB"`)
