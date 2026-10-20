package datamigrate

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type fakeRuntime struct {
	docker.Runtime
	mu         sync.Mutex
	running    bool
	created    []docker.ContainerSpec
	removed    []string
	dumpErr    error
	dumpOutput string
	srcCounts  string
	tgtCounts  string
	tgtFails   int
}

func (f *fakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	return &docker.ContainerState{ID: name, Name: name, Running: f.running}, nil
}

func (f *fakeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, spec)
	return "helper-1", nil
}

func (f *fakeRuntime) Start(context.Context, string) error { return nil }

func (f *fakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, id)
	return nil
}

type failingReader struct {
	data string
	err  error
	done bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), nil
}

func (r *failingReader) Close() error { return nil }

func (f *fakeRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	script := cmd[len(cmd)-1]
	dump, _ := DumpCommand("postgres")
	srcCount, _ := SourceCountCommand("postgres")
	switch script {
	case dump[2]:
		if f.dumpErr != nil {
			return &failingReader{data: f.dumpOutput, err: f.dumpErr}, nil
		}
		return io.NopCloser(strings.NewReader(f.dumpOutput)), nil
	case srcCount[2]:
		return io.NopCloser(strings.NewReader(f.srcCounts)), nil
	}
	if f.tgtFails > 0 {
		f.tgtFails--
		return nil, errors.New("engine still starting")
	}
	return io.NopCloser(strings.NewReader(f.tgtCounts)), nil
}

type fakeRestorer struct {
	got string
	err error
}

func (r *fakeRestorer) Restore(_ context.Context, _, _ string, dump io.Reader) error {
	b, _ := io.ReadAll(dump)
	r.got = string(b)
	return r.err
}

func newCopier(rt *fakeRuntime, rs *fakeRestorer) *Copier {
	return &Copier{Runtime: rt, Restorer: rs, VerifyAttempts: 3, VerifyInterval: time.Millisecond}
}

var pgTarget = Target{Name: "main", Engine: "postgres", Version: "16"}

func TestCopyVerifies(t *testing.T) {
	rt := &fakeRuntime{running: true, dumpOutput: "CREATE TABLE t();", srcCounts: "public.users|5\n", tgtCounts: "public.users|5\n", tgtFails: 1}
	rs := &fakeRestorer{}
	v, err := newCopier(rt, rs).Copy(t.Context(), pgTarget, Source{Host: "h", Port: 5432, User: "u", Password: "pw", Database: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Checked != 1 || v.Mismatched != 0 || rs.got != "CREATE TABLE t();" {
		t.Errorf("verification = %+v, restored %q", v, rs.got)
	}
	if len(rt.created) != 1 || rt.created[0].Image != "postgres:16" || rt.created[0].Env["SRC_PASSWORD"] != "pw" {
		t.Errorf("helper spec = %+v", rt.created)
	}
	if len(rt.removed) != 1 {
		t.Errorf("helper removed %d times, want 1", len(rt.removed))
	}
}

func TestCopyFailures(t *testing.T) {
	src := Source{Host: "h", Port: 5432, User: "u", Password: "hunter2", Database: "d"}
	tests := []struct {
		name    string
		rt      *fakeRuntime
		rs      *fakeRestorer
		wantErr string
		helper  bool
	}{
		{
			name:    "target not running",
			rt:      &fakeRuntime{running: false},
			rs:      &fakeRestorer{},
			wantErr: "not running yet",
		},
		{
			name:    "dump dies half way",
			rt:      &fakeRuntime{running: true, dumpOutput: "CREATE TABLE half", dumpErr: errors.New("exec exited 1: connection to server at hunter2 lost")},
			rs:      &fakeRestorer{},
			wantErr: "dump of the source failed",
			helper:  true,
		},
		{
			name:    "restore fails",
			rt:      &fakeRuntime{running: true, dumpOutput: "x"},
			rs:      &fakeRestorer{err: errors.New("psql: syntax error")},
			wantErr: "restore into the managed database",
			helper:  true,
		},
		{
			name:    "row counts differ",
			rt:      &fakeRuntime{running: true, dumpOutput: "x", srcCounts: "public.a|10\npublic.b|2\n", tgtCounts: "public.a|10\npublic.b|1\n"},
			rs:      &fakeRestorer{},
			wantErr: "1 of 2 tables differ",
			helper:  true,
		},
		{
			name:    "target never answers",
			rt:      &fakeRuntime{running: true, dumpOutput: "x", srcCounts: "public.a|1\n", tgtFails: 99},
			rs:      &fakeRestorer{},
			wantErr: "count target rows",
			helper:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newCopier(tt.rt, tt.rs).Copy(t.Context(), pgTarget, src)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("error leaks the source password: %v", err)
			}
			if tt.helper && len(tt.rt.removed) != 1 {
				t.Errorf("helper container removed %d times, want 1", len(tt.rt.removed))
			}
			if !tt.helper && len(tt.rt.created) != 0 {
				t.Error("a helper was created although the target was not ready")
			}
		})
	}
}

func TestCopyUnsupportedEngine(t *testing.T) {
	_, err := newCopier(&fakeRuntime{running: true}, &fakeRestorer{}).Copy(t.Context(), Target{Name: "c", Engine: "clickhouse"}, Source{Host: "h"})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err = %v", err)
	}
}
