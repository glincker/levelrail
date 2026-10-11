package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// memObjects is an in-memory bucket usable as Uploader, Downloader,
// ObjectProber and Deleter.
type memObjects struct {
	mu          sync.Mutex
	objects     map[string][]byte
	failUploads int
	failAfter   int64
	uploads     int
}

func newMemObjects() *memObjects { return &memObjects{objects: map[string][]byte{}} }

func (m *memObjects) Upload(ctx context.Context, _ Destination, key string, r io.Reader, _ int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	m.uploads++
	fail := m.failUploads > 0
	if fail {
		m.failUploads--
	}
	m.mu.Unlock()
	if fail {
		_, _ = io.CopyN(io.Discard, r, m.failAfter)
		return errors.New("simulated upload failure mid-object")
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return err
	}
	m.mu.Lock()
	m.objects[key] = buf.Bytes()
	m.mu.Unlock()
	return nil
}

func (m *memObjects) Download(_ context.Context, _ Destination, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), b...))), nil
}

func (m *memObjects) Head(_ context.Context, _ Destination, key string) (ObjectInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	return ObjectInfo{Exists: ok, Size: int64(len(b))}, nil
}

func (m *memObjects) Delete(_ context.Context, _ Destination, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *memObjects) flip(key string, at int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key][at] ^= 0xff
}

func (m *memObjects) truncate(key string, n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = m.objects[key][:n]
}

// memVolumes is an in-memory set of Docker volumes usable as VolumeArchiver,
// VolumeRestorer, VolumeCreator and VolumeRemover.
type memVolumes struct {
	mu      sync.Mutex
	vols    map[string]map[string]memFile
	removed []string
}

type memFile struct {
	data []byte
	mode int64
	uid  int
	link string
}

func newMemVolumes() *memVolumes { return &memVolumes{vols: map[string]map[string]memFile{}} }

func (v *memVolumes) put(vol, path string, f memFile) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.vols[vol] == nil {
		v.vols[vol] = map[string]memFile{}
	}
	v.vols[vol][path] = f
}

func (v *memVolumes) Archive(_ context.Context, vol string) (io.ReadCloser, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	files, ok := v.vols[vol]
	if !ok {
		return nil, fmt.Errorf("volume %q does not exist", vol)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f := files[n]
		h := &tar.Header{Name: "./" + n, Mode: f.mode, Uid: f.uid, Typeflag: tar.TypeReg, Size: int64(len(f.data))}
		if f.link != "" {
			h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, f.link, 0
		}
		_ = tw.WriteHeader(h)
		if h.Typeflag == tar.TypeReg {
			_, _ = tw.Write(f.data)
		}
	}
	_ = tw.Close()
	return io.NopCloser(&buf), nil
}

func (v *memVolumes) Restore(_ context.Context, vol string, archive io.Reader) error {
	files := map[string]memFile{}
	tr := tar.NewReader(archive)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(h.Name, "./")
		if h.Typeflag == tar.TypeSymlink {
			files[name] = memFile{mode: h.Mode, uid: h.Uid, link: h.Linkname}
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		files[name] = memFile{data: b, mode: h.Mode, uid: h.Uid}
	}
	if _, err := io.Copy(io.Discard, archive); err != nil {
		return err
	}
	v.mu.Lock()
	v.vols[vol] = files
	v.mu.Unlock()
	return nil
}

func (v *memVolumes) EnsureVolume(_ context.Context, name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.vols[name] == nil {
		v.vols[name] = map[string]memFile{}
	}
	return nil
}

func (v *memVolumes) RemoveVolume(_ context.Context, name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.vols, name)
	v.removed = append(v.removed, name)
	return nil
}

func (v *memVolumes) exists(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.vols[name]
	return ok
}

// memHistory is an in-memory history, drill and protection store.
type memHistory struct {
	mu       sync.Mutex
	target   store.BackupTarget
	rows     map[string]store.BackupHistory
	drills   map[string]store.BackupDrill
	policies map[string]store.VolumeBackupPolicy
	order    []string
	finishAt map[string]string
	noSeal   bool
}

func newMemHistory() *memHistory {
	return &memHistory{
		target:   newTestTarget(),
		rows:     map[string]store.BackupHistory{},
		drills:   map[string]store.BackupDrill{},
		policies: map[string]store.VolumeBackupPolicy{},
		finishAt: map[string]string{},
	}
}

func (h *memHistory) GetBackupTarget(context.Context, string) (store.BackupTarget, error) {
	return h.target, nil
}

func (h *memHistory) StartBackupHistory(_ context.Context, r store.BackupHistory) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r.Status = store.BackupStatusRunning
	h.rows[r.ID] = r
	h.order = append(h.order, r.ID)
	return nil
}

func (h *memHistory) FinishBackupHistory(_ context.Context, id, status string, size int64, checksum, errMsg, finishedAt string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rows[id]
	if !ok {
		return store.ErrBackupHistoryNotFound
	}
	r.Status, r.SizeBytes, r.ChecksumSHA256, r.Error, r.FinishedAt = status, size, checksum, errMsg, finishedAt
	h.rows[id] = r
	return nil
}

func (h *memHistory) GetBackupHistory(_ context.Context, id string) (store.BackupHistory, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rows[id]
	if !ok {
		return store.BackupHistory{}, store.ErrBackupHistoryNotFound
	}
	return r, nil
}

func (h *memHistory) SetBackupSealInfo(_ context.Context, id, codec, digest string, files, bytes int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.rows[id]
	r.Codec, r.ManifestDigest, r.ManifestFiles, r.ManifestBytes = codec, digest, files, bytes
	h.rows[id] = r
	return nil
}

func (h *memHistory) StartBackupDrill(_ context.Context, d store.BackupDrill) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	d.Status = store.DrillStatusRunning
	h.drills[d.ID] = d
	return nil
}

func (h *memHistory) FinishBackupDrill(_ context.Context, d store.BackupDrill) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drills[d.ID] = d
	return nil
}

func (h *memHistory) ListSucceededBackups(context.Context) ([]store.BackupHistory, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []store.BackupHistory
	for _, r := range h.rows {
		if r.Status == store.BackupStatusSucceeded {
			out = append(out, r)
		}
	}
	return out, nil
}

func (h *memHistory) ListSucceededVolumeBackups(_ context.Context, svc, vol string) ([]store.BackupHistory, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []store.BackupHistory
	for _, r := range h.rows {
		if r.Status == store.BackupStatusSucceeded && r.ServiceName == svc && r.VolumeName == vol {
			out = append(out, r)
		}
	}
	return out, nil
}

func (h *memHistory) DeleteBackupHistory(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rows, id)
	return nil
}

func (h *memHistory) ListBackupDrills(_ context.Context, svc, vol, db string, _ int) ([]store.BackupDrill, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []store.BackupDrill
	for _, d := range h.drills {
		if (svc == "" || d.ServiceName == svc) && (vol == "" || d.VolumeName == vol) && (db == "" || d.DatabaseName == db) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out, nil
}

func (h *memHistory) ListRunningBackups(_ context.Context, before time.Time) ([]store.BackupHistory, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []store.BackupHistory
	for _, r := range h.rows {
		if r.Status != store.BackupStatusRunning {
			continue
		}
		if at, err := time.Parse(time.RFC3339, r.StartedAt); err == nil && at.Before(before) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (h *memHistory) ListBackupTargets(context.Context) ([]store.BackupTarget, error) {
	return []store.BackupTarget{h.target}, nil
}

func (h *memHistory) SetBackupTargetProtection(context.Context, store.BackupTargetProtection) error {
	return nil
}

func (h *memHistory) GetVolumeBackupPolicy(_ context.Context, svc, vol string) (store.VolumeBackupPolicy, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.policies[svc+"/"+vol]
	if !ok {
		return store.VolumeBackupPolicy{}, store.ErrVolumeBackupPolicyNotFound
	}
	return p, nil
}

// noSealStore wraps memHistory hiding SealRecorder to prove the backup fails
// instead of producing an unrestorable object.
type noSealStore struct{ *memHistory }

func (noSealStore) SetBackupSealInfo() {}

func (h *memHistory) StartVolumeCloneRestore(context.Context, store.VolumeCloneRestore) error {
	return nil
}

func (h *memHistory) FinishVolumeCloneRestore(context.Context, string, string, string, string) error {
	return nil
}
