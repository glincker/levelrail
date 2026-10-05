package backup

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	seg1 = "000000010000000000000001"
	seg2 = "000000010000000000000002"
	seg3 = "000000010000000000000003"
)

type scriptWALRuntime struct {
	segBytes string
	listing  string
	files    map[string]string
	catFail  map[string]bool
}

func (r *scriptWALRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	joined := strings.Join(cmd, " ")
	switch {
	case strings.Contains(joined, "wal_segment_size"):
		return io.NopCloser(strings.NewReader(r.segBytes)), nil
	case strings.Contains(joined, "wc -c"):
		return io.NopCloser(strings.NewReader(r.listing)), nil
	case cmd[0] == "cat":
		name := cmd[1][strings.LastIndex(cmd[1], "/")+1:]
		if r.catFail[name] {
			return nil, errors.New("cat failed")
		}
		return io.NopCloser(strings.NewReader(r.files[name])), nil
	}
	return nil, errors.New("unexpected exec " + joined)
}

func (r *scriptWALRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}

type recordingUploader struct {
	mu   sync.Mutex
	keys []string
	body map[string]string
	err  error
}

func (u *recordingUploader) Upload(_ context.Context, _ Destination, key string, r io.Reader, _ int64) error {
	if u.err != nil {
		return u.err
	}
	b, _ := io.ReadAll(r)
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.body == nil {
		u.body = map[string]string{}
	}
	u.keys = append(u.keys, key)
	u.body[key] = string(b)
	return nil
}

type staticLister struct {
	objs []RemoteObject
	err  error
}

func (l staticLister) List(context.Context, Destination, string) ([]RemoteObject, error) {
	return l.objs, l.err
}

func TestWALShipper_Ship(t *testing.T) {
	full := "16777216"
	listing := strings.Join([]string{
		seg1 + " " + full,
		seg2 + " " + full,
		seg3 + " 4096",
		"00000002.history 40",
		"junk.tmp 12",
		"",
	}, "\n")
	tests := []struct {
		name     string
		remote   []RemoteObject
		wantKeys []string
	}{
		{
			name:     "ships complete segments and history, skips partial and junk",
			wantKeys: []string{"main/wal/" + seg1, "main/wal/" + seg2, "main/wal/00000002.history"},
		},
		{
			name:     "skips files the target already holds at the same size",
			remote:   []RemoteObject{{Key: "main/wal/" + seg1, Size: 16777216}, {Key: "main/wal/00000002.history", Size: 40}},
			wantKeys: []string{"main/wal/" + seg2},
		},
		{
			name:     "reuploads a remote copy with the wrong size",
			remote:   []RemoteObject{{Key: "main/wal/" + seg1, Size: 100}, {Key: "main/wal/" + seg2, Size: 16777216}, {Key: "main/wal/00000002.history", Size: 40}},
			wantKeys: []string{"main/wal/" + seg1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &scriptWALRuntime{segBytes: "16777216\n", listing: listing, files: map[string]string{seg1: "a", seg2: "b", "00000002.history": "h"}}
			up := &recordingUploader{}
			s := &WALShipper{Runtime: rt, Uploader: up, Lister: staticLister{objs: tt.remote}}
			n, err := s.Ship(context.Background(), Destination{Bucket: "b"}, "main", "db-main")
			if err != nil {
				t.Fatalf("Ship() error = %v", err)
			}
			if !reflect.DeepEqual(up.keys, tt.wantKeys) || n != len(tt.wantKeys) {
				t.Errorf("uploaded %v (n=%d), want %v", up.keys, n, tt.wantKeys)
			}
		})
	}
}

func TestWALShipper_ShipErrors(t *testing.T) {
	listing := seg1 + " 16777216\n"
	t.Run("read failure stops the pass", func(t *testing.T) {
		rt := &scriptWALRuntime{segBytes: "16777216", listing: listing, catFail: map[string]bool{seg1: true}}
		s := &WALShipper{Runtime: rt, Uploader: &recordingUploader{}, Lister: staticLister{}}
		if _, err := s.Ship(context.Background(), Destination{}, "main", "db-main"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("upload failure is returned", func(t *testing.T) {
		rt := &scriptWALRuntime{segBytes: "16777216", listing: listing, files: map[string]string{seg1: "x"}}
		s := &WALShipper{Runtime: rt, Uploader: &recordingUploader{err: errors.New("boom")}, Lister: staticLister{}}
		if _, err := s.Ship(context.Background(), Destination{}, "main", "db-main"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("list failure is returned", func(t *testing.T) {
		rt := &scriptWALRuntime{segBytes: "16777216", listing: listing}
		s := &WALShipper{Runtime: rt, Uploader: &recordingUploader{}, Lister: staticLister{err: errors.New("denied")}}
		if _, err := s.Ship(context.Background(), Destination{}, "main", "db-main"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("bad segment size output", func(t *testing.T) {
		rt := &scriptWALRuntime{segBytes: "oops", listing: listing}
		s := &WALShipper{Runtime: rt, Uploader: &recordingUploader{}, Lister: staticLister{}}
		if _, err := s.Ship(context.Background(), Destination{}, "main", "db-main"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestSegmentOfLSN(t *testing.T) {
	tests := []struct {
		lsn     string
		want    string
		wantErr bool
	}{
		{"0/3000100", "0000000000000003", false},
		{"1/FF000000", "00000001000000FF", false},
		{"0/0", "0000000000000000", false},
		{"nonsense", "", true},
		{"zz/1", "", true},
	}
	for _, tt := range tests {
		got, err := segmentOfLSN(tt.lsn, defaultWALSegmentBytes)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("segmentOfLSN(%q) = %q, %v; want %q (err %v)", tt.lsn, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestPruneRemoteWAL(t *testing.T) {
	objs := []RemoteObject{
		{Key: "main/wal/" + seg1}, {Key: "main/wal/" + seg2}, {Key: "main/wal/" + seg3},
		{Key: "main/wal/00000002.history"},
		{Key: "main/wal/" + seg1 + ".00000028.backup"}, {Key: "main/wal/" + seg3 + ".00000028.backup"},
	}
	del := &fakeDeleter{}
	n, err := PruneRemoteWAL(context.Background(), Destination{Bucket: "b"}, staticLister{objs: objs}, del, "main", "0/2000100")
	if err != nil {
		t.Fatalf("PruneRemoteWAL() error = %v", err)
	}
	if n != 2 || len(del.calls) != 2 || del.calls[0].key != "main/wal/"+seg1 || del.calls[1].key != "main/wal/"+seg1+".00000028.backup" {
		t.Errorf("removed %d, calls %+v; want the old segment and its label only", n, del.calls)
	}
	if _, err := PruneRemoteWAL(context.Background(), Destination{}, staticLister{}, del, "main", "bad"); err == nil {
		t.Error("malformed LSN must error")
	}
}

type fakeShipper struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeShipper) Ship(_ context.Context, _ Destination, db, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, db)
	return 2, f.err
}

type fakeShipStore struct {
	dbs     []store.DesiredDatabase
	history map[string][]store.BaseBackupHistory
}

func (f fakeShipStore) ListDesiredDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return f.dbs, nil
}

func (f fakeShipStore) ListBaseBackupHistory(_ context.Context, name string) ([]store.BaseBackupHistory, error) {
	return f.history[name], nil
}

func TestWALShipScheduler_Tick(t *testing.T) {
	okHist := []store.BaseBackupHistory{{ID: "b2", Status: store.BackupStatusFailed, TargetID: "bad"}, {ID: "b1", Status: store.BackupStatusSucceeded, TargetID: "tgt"}}
	st := fakeShipStore{
		dbs: []store.DesiredDatabase{
			{Name: "on", Engine: store.EnginePostgres, PITREnabled: true},
			{Name: "off", Engine: store.EnginePostgres},
			{Name: "remote", Engine: store.EnginePostgres, PITREnabled: true, NodeID: "n2"},
			{Name: "nobase", Engine: store.EnginePostgres, PITREnabled: true},
		},
		history: map[string][]store.BaseBackupHistory{"on": okHist, "remote": okHist},
	}
	sh := &fakeShipper{}
	var gotTarget string
	s := &WALShipScheduler{
		Store: st, Shipper: sh,
		Resolve:       func(_ context.Context, id string) (Destination, error) { gotTarget = id; return Destination{}, nil },
		ContainerName: func(n string) string { return "db-" + n },
		IsLocal:       func(node string) bool { return node == "" },
	}
	s.Tick(context.Background())
	if !reflect.DeepEqual(sh.calls, []string{"on"}) {
		t.Fatalf("shipped %v, want [on]", sh.calls)
	}
	if gotTarget != "tgt" {
		t.Errorf("target = %q, want the newest succeeded base backup's target", gotTarget)
	}
	status, ok := s.WALShipStatus("on")
	if !ok || status.Shipped != 2 || status.LastError != "" || status.LastSuccessAt.IsZero() {
		t.Errorf("status = %+v, %v", status, ok)
	}

	sh.err = errors.New("bucket down")
	s.Tick(context.Background())
	status, _ = s.WALShipStatus("on")
	if status.LastError == "" || status.Shipped != 2 {
		t.Errorf("after failure status = %+v, want LastError set and Shipped unchanged", status)
	}
}
