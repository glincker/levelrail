package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type memDrillStorage struct {
	objs    map[string][]byte
	corrupt bool
	failPut bool
	failGet bool
	deleted int
}

func (m *memDrillStorage) Upload(_ context.Context, _ Destination, key string, r io.Reader, _ int64) error {
	if m.failPut {
		return errors.New("denied")
	}
	b, _ := io.ReadAll(r)
	if m.corrupt {
		b = append([]byte("x"), b[1:]...)
	}
	m.objs[key] = b
	return nil
}

func (m *memDrillStorage) Download(_ context.Context, _ Destination, key string) (io.ReadCloser, error) {
	if m.failGet {
		return nil, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(m.objs[key])), nil
}

func (m *memDrillStorage) Delete(_ context.Context, _ Destination, key string) error {
	delete(m.objs, key)
	m.deleted++
	return nil
}

func TestRunDrill(t *testing.T) {
	cases := []struct {
		name      string
		st        *memDrillStorage
		wantOK    bool
		wantStage string
	}{
		{"round trip passes and cleans up", &memDrillStorage{objs: map[string][]byte{}}, true, ""},
		{"upload failure is reported", &memDrillStorage{objs: map[string][]byte{}, failPut: true}, false, "upload"},
		{"download failure still cleans up", &memDrillStorage{objs: map[string][]byte{}, failGet: true}, false, "download"},
		{"corrupt readback fails the compare", &memDrillStorage{objs: map[string][]byte{}, corrupt: true}, false, "compare"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := runDrill(context.Background(), tc.st, Destination{}, 64)
			if rep.OK != tc.wantOK || rep.Stage != tc.wantStage {
				t.Fatalf("report = %+v, want ok=%v stage=%q", rep, tc.wantOK, tc.wantStage)
			}
			if tc.wantStage != "upload" && (!rep.CleanedUp || len(tc.st.objs) != 0) {
				t.Errorf("sample object left behind: %+v", rep)
			}
			for k := range tc.st.objs {
				if !strings.HasPrefix(k, DrillObjectPrefix) {
					t.Errorf("key %q outside the drill prefix", k)
				}
			}
		})
	}
}
