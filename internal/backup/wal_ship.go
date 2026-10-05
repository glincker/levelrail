package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// envPITRWALSegmentBytes overrides the WAL segment size the fetcher assumes
// when mapping a base backup's start LSN to a segment (Postgres default 16MiB).
const envPITRWALSegmentBytes = "APP_PITR_WAL_SEGMENT_BYTES"

const defaultWALSegmentBytes int64 = 16 << 20

var (
	walSegmentRe = regexp.MustCompile(`^[0-9A-F]{24}$`)
	walAuxRe     = regexp.MustCompile(`^([0-9A-F]{8}\.history|[0-9A-F]{24}\.[0-9A-F]{8}\.backup)$`)
)

// WALPrefix is the key prefix under which a database's archived WAL lives in
// its backup target.
func WALPrefix(databaseName string) string { return databaseName + "/wal/" }

func isShippableWALName(name string) bool {
	return walSegmentRe.MatchString(name) || walAuxRe.MatchString(name)
}

// WALShipper copies a database's local WAL archive into its backup target so
// point-in-time restore survives losing the database host's disk.
type WALShipper struct {
	Runtime  Runtime
	Uploader Uploader
	Lister   Lister
	Logger   *slog.Logger
}

// Ship uploads every complete archived segment the target does not already
// hold at the same size, and returns how many it uploaded. A segment still
// being copied by archive_command is skipped because it is not yet full size.
func (s *WALShipper) Ship(ctx context.Context, dest Destination, databaseName, containerName string) (int, error) {
	segBytes, err := walSegmentSize(ctx, s.Runtime, containerName)
	if err != nil {
		return 0, err
	}
	local, err := listLocalWAL(ctx, s.Runtime, containerName)
	if err != nil {
		return 0, err
	}
	remoteObjs, err := s.Lister.List(ctx, dest, WALPrefix(databaseName))
	if err != nil {
		return 0, fmt.Errorf("list shipped wal for %q: %w", databaseName, err)
	}
	remote := make(map[string]int64, len(remoteObjs))
	for _, o := range remoteObjs {
		remote[strings.TrimPrefix(o.Key, WALPrefix(databaseName))] = o.Size
	}

	names := make([]string, 0, len(local))
	for n := range local {
		names = append(names, n)
	}
	sort.Strings(names)

	shipped := 0
	for _, name := range names {
		size := local[name]
		if walSegmentRe.MatchString(name) && size != segBytes {
			continue
		}
		if remote[name] == size {
			continue
		}
		if err := s.shipOne(ctx, dest, databaseName, containerName, name, size); err != nil {
			return shipped, err
		}
		shipped++
	}
	return shipped, nil
}

func (s *WALShipper) shipOne(ctx context.Context, dest Destination, databaseName, containerName, name string, size int64) error {
	rc, err := s.Runtime.Exec(ctx, containerName, []string{"cat", postgresWALArchivePath + "/" + name})
	if err != nil {
		return fmt.Errorf("read wal %q: %w", name, err)
	}
	defer func() { _ = rc.Close() }()
	if err := s.Uploader.Upload(ctx, dest, WALPrefix(databaseName)+name, rc, size); err != nil {
		return fmt.Errorf("ship wal %q of %q: %w", name, databaseName, err)
	}
	return nil
}

func walSegmentSize(ctx context.Context, rt Runtime, containerName string) (int64, error) {
	out, err := execOutput(ctx, rt, containerName, []string{"sh", "-c", `exec psql --no-password -U "$POSTGRES_USER" -Atq -c "SELECT pg_size_bytes(current_setting('wal_segment_size'))"`})
	if err != nil {
		return 0, fmt.Errorf("query wal segment size: %w", err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("query wal segment size: unexpected output %q", out)
	}
	return n, nil
}

func listLocalWAL(ctx context.Context, rt Runtime, containerName string) (map[string]int64, error) {
	script := `cd ` + postgresWALArchivePath + ` && for f in *; do [ -f "$f" ] && echo "$f $(wc -c < "$f")"; done; exit 0`
	out, err := execOutput(ctx, rt, containerName, []string{"sh", "-c", script})
	if err != nil {
		return nil, fmt.Errorf("list local wal archive: %w", err)
	}
	local := map[string]int64{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || !isShippableWALName(f[0]) {
			continue
		}
		n, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		local[f[0]] = n
	}
	return local, nil
}

func execOutput(ctx context.Context, rt Runtime, containerName string, cmd []string) (string, error) {
	rc, err := rt.Exec(ctx, containerName, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// segmentOfLSN maps "X/Y" to the 16 hex digit segment number (log id and
// segment id, without the timeline), the part of a WAL file name that orders.
func segmentOfLSN(lsn string, segBytes int64) (string, error) {
	hi, lo, ok := strings.Cut(lsn, "/")
	if !ok {
		return "", fmt.Errorf("malformed LSN %q", lsn)
	}
	x, err := strconv.ParseUint(hi, 16, 32)
	if err != nil {
		return "", fmt.Errorf("malformed LSN %q: %w", lsn, err)
	}
	y, err := strconv.ParseUint(lo, 16, 32)
	if err != nil {
		return "", fmt.Errorf("malformed LSN %q: %w", lsn, err)
	}
	if segBytes <= 0 {
		return "", fmt.Errorf("invalid wal segment size %d", segBytes)
	}
	return fmt.Sprintf("%08X%08X", x, y/uint64(segBytes)), nil //nolint:gosec // segBytes checked positive above
}

// WALFetcher restores shipped WAL from a backup target into a database's
// local archive volume, so recovery works after the local copy is lost.
type WALFetcher struct {
	Runtime    docker.Runtime
	Lister     Lister
	Downloader Downloader
}

func walSegmentBytesFromEnv() int64 {
	if v := os.Getenv(envPITRWALSegmentBytes); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultWALSegmentBytes
}

// Fetch copies into walVolume every shipped file not older than the segment
// holding sinceLSN (empty fetches everything), overwriting same-named local
// files: after a host loss a re-initialised cluster reuses segment names, and
// the shipped copy is the lineage the base backup belongs to. It then fixes
// ownership for the postgres user and returns how many files it fetched.
func (f *WALFetcher) Fetch(ctx context.Context, dest Destination, databaseName, walVolume, sinceLSN string) (int, error) {
	remoteObjs, err := f.Lister.List(ctx, dest, WALPrefix(databaseName))
	if err != nil {
		return 0, fmt.Errorf("list shipped wal for %q: %w", databaseName, err)
	}
	if len(remoteObjs) == 0 {
		return 0, nil
	}
	floor := ""
	if sinceLSN != "" {
		if floor, err = segmentOfLSN(sinceLSN, walSegmentBytesFromEnv()); err != nil {
			return 0, err
		}
	}

	id, err := createVolumeHelper(ctx, f.Runtime, walVolume, "walfetch", false)
	if err != nil {
		return 0, fmt.Errorf("open wal volume %q: %w", walVolume, err)
	}
	defer func() { _ = f.Runtime.Remove(context.Background(), id, true) }()

	fetched := 0
	for _, o := range remoteObjs {
		name := strings.TrimPrefix(o.Key, WALPrefix(databaseName))
		if !isShippableWALName(name) {
			continue
		}
		if walSegmentRe.MatchString(name) && name[8:] < floor {
			continue
		}
		if err := f.fetchOne(ctx, dest, id, o.Key, name); err != nil {
			return fetched, err
		}
		fetched++
	}
	chown := fmt.Sprintf("chown -R %d:%d %s", postgresContainerUID, postgresContainerGID, volumeMountPath)
	if _, err := execOutput(ctx, f.Runtime, id, []string{"sh", "-c", chown}); err != nil {
		return fetched, fmt.Errorf("chown wal volume %q: %w", walVolume, err)
	}
	return fetched, nil
}

func (f *WALFetcher) fetchOne(ctx context.Context, dest Destination, helperID, key, name string) error {
	body, err := f.Downloader.Download(ctx, dest, key)
	if err != nil {
		return fmt.Errorf("download wal %q: %w", name, err)
	}
	defer func() { _ = body.Close() }()
	tmp := volumeMountPath + "/." + name + ".tmp"
	cmd := []string{"sh", "-c", fmt.Sprintf("cat > %s && mv %s %s/%s", tmp, tmp, volumeMountPath, name)}
	rc, err := f.Runtime.ExecWithInput(ctx, helperID, cmd, body)
	if err != nil {
		return fmt.Errorf("write wal %q: %w", name, err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("write wal %q: %w", name, err)
	}
	return nil
}

// PruneRemoteWAL deletes shipped segments older than the segment holding
// startLSN, mirroring PruneWALArchive for the copy in the backup target.
func PruneRemoteWAL(ctx context.Context, dest Destination, lister Lister, deleter Deleter, databaseName, startLSN string) (int, error) {
	floor, err := segmentOfLSN(startLSN, walSegmentBytesFromEnv())
	if err != nil {
		return 0, err
	}
	objs, err := lister.List(ctx, dest, WALPrefix(databaseName))
	if err != nil {
		return 0, fmt.Errorf("list shipped wal for %q: %w", databaseName, err)
	}
	removed := 0
	for _, o := range objs {
		name := strings.TrimPrefix(o.Key, WALPrefix(databaseName))
		if !walSegmentRe.MatchString(name) || name[8:] >= floor {
			continue
		}
		if err := deleter.Delete(ctx, dest, o.Key); err != nil {
			return removed, fmt.Errorf("delete shipped wal %q: %w", o.Key, err)
		}
		removed++
	}
	return removed, nil
}
