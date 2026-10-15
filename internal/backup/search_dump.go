package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultSearchTaskTimeout  = 30 * time.Minute
	defaultSearchPollInterval = 2 * time.Second
)

// errSearchTaskTimeout marks a snapshot the engine had not finished in time.
var errSearchTaskTimeout = errors.New("timed out waiting for the engine to finish the snapshot")

func (d *ContainerDumper) searchTiming() (timeout, interval time.Duration) {
	timeout, interval = d.SearchTaskTimeout, d.SearchPollInterval
	if timeout <= 0 {
		timeout = defaultSearchTaskTimeout
	}
	if interval <= 0 {
		interval = defaultSearchPollInterval
	}
	return timeout, interval
}

// dumpSearch snapshots a search engine through its own API, then streams
// the artifact out, failing the stream unless every byte and the SHA-256
// match what the file held on disk.
func (d *ContainerDumper) dumpSearch(ctx context.Context, engine, containerName string) (io.ReadCloser, error) {
	before, beforeErr := ReadSearchContent(ctx, d.Runtime, engine, containerName)

	path, cleanup, err := d.createSearchArtifact(ctx, engine, containerName)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, fmt.Errorf("backup: dump %s container %q: %w", engine, containerName, err)
	}

	summary := ""
	if after, afterErr := ReadSearchContent(ctx, d.Runtime, engine, containerName); beforeErr == nil && afterErr == nil && before.Diff(after) == "" {
		summary = before.Encode()
	}

	stream, err := d.streamVerifiedFile(ctx, containerName, path, cleanup)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("backup: dump %s container %q: %w", engine, containerName, err)
	}
	stream.summary = summary
	return stream, nil
}

// createSearchArtifact asks the engine for a snapshot and returns its file
// path inside the container plus a best-effort cleanup.
func (d *ContainerDumper) createSearchArtifact(ctx context.Context, engine, containerName string) (string, func(), error) {
	l, _ := database.SearchEngineLayout(engine)
	switch engine {
	case store.EngineMeilisearch:
		var task struct {
			TaskUID int64 `json:"taskUid"`
		}
		if err := searchAPI(ctx, d.Runtime, engine, containerName, "POST", "/dumps", "", &task); err != nil {
			return "", nil, fmt.Errorf("create dump: %w", err)
		}
		uid, err := d.waitMeilisearchTask(ctx, containerName, task.TaskUID)
		if err != nil {
			return "", nil, err
		}
		path := l.VolumePath + "/dumps/" + uid + ".dump"
		return path, d.execCleanup(containerName, []string{"rm", "-f", path}), nil

	case store.EngineQdrant:
		var snap struct {
			Result struct {
				Name string `json:"name"`
			} `json:"result"`
		}
		timeout, _ := d.searchTiming()
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := searchAPI(callCtx, d.Runtime, engine, containerName, "POST", "/snapshots?wait=true", "", &snap); err != nil {
			if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
				return "", nil, errSearchTaskTimeout
			}
			return "", nil, fmt.Errorf("create snapshot: %w", err)
		}
		name := snap.Result.Name
		if !safeArtifactName.MatchString(name) {
			return "", nil, fmt.Errorf("engine reported an unusable snapshot name %q", name)
		}
		cleanup := func() {
			cctx, ccancel := context.WithTimeout(context.Background(), time.Minute)
			defer ccancel()
			_ = searchAPI(cctx, d.Runtime, engine, containerName, "DELETE", "/snapshots/"+name, "", nil)
		}
		path, err := d.findFile(ctx, containerName, l.VolumePath+"/snapshots", name)
		return path, cleanup, err

	case store.EngineTypesense:
		dir := l.VolumePath + "/.backup-snapshot"
		tarPath := l.VolumePath + "/.backup-snapshot.tar"
		cleanup := d.execCleanup(containerName, []string{"rm", "-rf", dir, tarPath})
		if err := d.drain(ctx, containerName, []string{"rm", "-rf", dir, tarPath}); err != nil {
			return "", cleanup, fmt.Errorf("clear previous snapshot: %w", err)
		}
		var res struct {
			Success bool `json:"success"`
		}
		timeout, _ := d.searchTiming()
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := searchAPI(callCtx, d.Runtime, engine, containerName, "POST", "/operations/snapshot?snapshot_path="+url.QueryEscape(dir), "", &res); err != nil {
			if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
				return "", cleanup, errSearchTaskTimeout
			}
			return "", cleanup, fmt.Errorf("create snapshot: %w", err)
		}
		if !res.Success {
			return "", cleanup, errors.New("engine reported the snapshot as unsuccessful")
		}
		pack := []string{"sh", "-c", `set -e; [ -n "$(ls -A "$0")" ] || { echo "snapshot directory is empty" >&2; exit 1; }; tar -C "$0" -cf "$1" .`, dir, tarPath}
		if err := d.drain(ctx, containerName, pack); err != nil {
			return "", cleanup, fmt.Errorf("pack snapshot: %w", err)
		}
		return tarPath, cleanup, nil
	}
	return "", nil, fmt.Errorf("engine %q has no snapshot API", engine)
}

// waitMeilisearchTask polls a dump task until it succeeds and returns the
// dump's uid. A failed, canceled or still-running task fails the backup.
func (d *ContainerDumper) waitMeilisearchTask(ctx context.Context, containerName string, uid int64) (string, error) {
	timeout, interval := d.searchTiming()
	deadline := time.Now().Add(timeout)
	for {
		var task struct {
			Status  string `json:"status"`
			Details struct {
				DumpUID string `json:"dumpUid"`
			} `json:"details"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		path := "/tasks/" + strconv.FormatInt(uid, 10)
		if err := searchAPI(ctx, d.Runtime, store.EngineMeilisearch, containerName, "GET", path, "", &task); err != nil {
			return "", fmt.Errorf("poll dump task %d: %w", uid, err)
		}
		switch task.Status {
		case "succeeded":
			if !safeArtifactName.MatchString(task.Details.DumpUID) {
				return "", fmt.Errorf("dump task %d succeeded with an unusable dump uid %q", uid, task.Details.DumpUID)
			}
			return task.Details.DumpUID, nil
		case "failed", "canceled":
			msg := task.Status
			if task.Error != nil && task.Error.Message != "" {
				msg += ": " + task.Error.Message
			}
			return "", fmt.Errorf("dump task %d %s", uid, msg)
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("dump task %d still %q: %w", uid, task.Status, errSearchTaskTimeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (d *ContainerDumper) findFile(ctx context.Context, containerName, dir, name string) (string, error) {
	out, err := d.capture(ctx, containerName, []string{"find", dir, "-maxdepth", "2", "-type", "f", "-name", name})
	if err != nil {
		return "", fmt.Errorf("locate snapshot %q: %w", name, err)
	}
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) != 1 {
		return "", fmt.Errorf("expected exactly one snapshot file named %q under %s, found %d", name, dir, len(lines))
	}
	return lines[0], nil
}

// streamVerifiedFile records the file's size and SHA-256 inside the
// container, then streams it; the stream errors at EOF on any mismatch.
func (d *ContainerDumper) streamVerifiedFile(ctx context.Context, containerName, path string, cleanup func()) (*verifiedStream, error) {
	out, err := d.capture(ctx, containerName, []string{"sh", "-c", `stat -c %s "$0" && sha256sum "$0"`, path})
	if err != nil {
		return nil, fmt.Errorf("measure %s: %w", path, err)
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return nil, fmt.Errorf("measure %s: unexpected output %q", path, strings.TrimSpace(out))
	}
	size, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || size <= 0 {
		return nil, fmt.Errorf("measure %s: snapshot is empty or unreadable (%q)", path, fields[0])
	}
	sum := strings.ToLower(fields[1])
	if len(sum) != sha256.Size*2 {
		return nil, fmt.Errorf("measure %s: unexpected checksum %q", path, sum)
	}
	rc, err := d.Runtime.Exec(ctx, containerName, []string{"cat", path})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return &verifiedStream{rc: rc, want: size, wantSum: sum, hash: sha256.New(), cleanup: cleanup}, nil
}

// verifiedStream refuses to report a clean EOF for a short or altered file,
// so a truncated artifact fails the upload instead of being stored.
type verifiedStream struct {
	rc      io.ReadCloser
	want    int64
	wantSum string
	n       int64
	hash    hash.Hash
	cleanup func()
	summary string
}

func (s *verifiedStream) Read(p []byte) (int, error) {
	n, err := s.rc.Read(p)
	s.n += int64(n)
	s.hash.Write(p[:n])
	if s.n > s.want {
		return n, fmt.Errorf("snapshot grew while streaming: read %d bytes, expected %d", s.n, s.want)
	}
	if errors.Is(err, io.EOF) {
		if s.n != s.want {
			return n, fmt.Errorf("snapshot truncated: read %d of %d bytes", s.n, s.want)
		}
		if got := hex.EncodeToString(s.hash.Sum(nil)); got != s.wantSum {
			return n, fmt.Errorf("snapshot checksum mismatch: streamed %s, file had %s", got, s.wantSum)
		}
	}
	return n, err
}

func (s *verifiedStream) Close() error {
	err := s.rc.Close()
	if s.cleanup != nil {
		s.cleanup()
	}
	return err
}

// ContentSummary is the stable object counts captured around the snapshot,
// "" when they changed while it was taken.
func (s *verifiedStream) ContentSummary() string { return s.summary }

func (d *ContainerDumper) execCleanup(containerName string, cmd []string) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = d.drain(ctx, containerName, cmd)
	}
}

func (d *ContainerDumper) drain(ctx context.Context, containerName string, cmd []string) error {
	_, err := d.capture(ctx, containerName, cmd)
	return err
}

func (d *ContainerDumper) capture(ctx context.Context, containerName string, cmd []string) (string, error) {
	return execCapture(ctx, d.Runtime, containerName, cmd)
}

// execCapture runs cmd and returns its stdout, surfacing a non-zero exit.
func execCapture(ctx context.Context, rt docker.Runtime, containerName string, cmd []string) (string, error) {
	rc, err := rt.Exec(ctx, containerName, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(io.LimitReader(rc, maxSearchAPIResponse))
	return string(out), err
}
