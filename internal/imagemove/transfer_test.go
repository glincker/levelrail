package imagemove

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

const (
	srcID   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	otherID = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

type fakeSaver struct {
	data     []byte
	saveErr  error
	closeErr error
	calls    int
	gotRef   string
}

func (f *fakeSaver) Save(_ context.Context, ref string) (io.ReadCloser, error) {
	f.calls++
	f.gotRef = ref
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return &fakeStream{Reader: bytes.NewReader(f.data), closeErr: f.closeErr}, nil
}

type fakeStream struct {
	io.Reader
	closeErr error
}

func (s *fakeStream) Close() error { return s.closeErr }

type fakeRuntime struct {
	images  map[string]string
	loadID  string
	loadErr error
	loaded  []byte
}

func (f *fakeRuntime) LoadImage(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	f.loaded = b
	if err != nil {
		return err
	}
	if f.loadErr != nil {
		return f.loadErr
	}
	return nil
}

func (f *fakeRuntime) InspectImageID(_ context.Context, ref string) (string, error) {
	if f.loaded != nil && f.loadErr == nil && f.loadID != "" {
		return f.loadID, nil
	}
	return f.images[ref], nil
}

func TestTransfer(t *testing.T) {
	payload := []byte(strings.Repeat("x", 4096))
	cases := []struct {
		name       string
		saver      *fakeSaver
		rt         *fakeRuntime
		wantID     string
		max        int64
		wantErr    string
		wantSaves  int
		wantVerify bool
		wantSkip   bool
	}{
		{name: "streams and verifies", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{loadID: srcID}, wantID: srcID, wantSaves: 1, wantVerify: true},
		{name: "already present skips the stream", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{images: map[string]string{"app:1": srcID}}, wantID: srcID, wantSaves: 0, wantVerify: true, wantSkip: true},
		{name: "stale tag on target is replaced", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{images: map[string]string{"app:1": otherID}, loadID: srcID}, wantID: srcID, wantSaves: 1, wantVerify: true},
		{name: "id mismatch fails", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{loadID: otherID}, wantID: srcID, wantSaves: 1, wantErr: "has ID"},
		{name: "no source id loads unverified", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{loadID: otherID}, wantSaves: 1},
		{name: "remote failure surfaces stderr", saver: &fakeSaver{closeErr: errors.New("docker save on the source failed: No such image")}, rt: &fakeRuntime{loadErr: errors.New("empty archive")}, wantID: srcID, wantSaves: 1, wantErr: "No such image"},
		{name: "load failure", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{loadErr: errors.New("disk full")}, wantID: srcID, wantSaves: 1, wantErr: "disk full"},
		{name: "size bound", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{loadID: srcID}, wantID: srcID, max: 1000, wantSaves: 1, wantErr: "size limit"},
		{name: "image missing after load", saver: &fakeSaver{data: payload}, rt: &fakeRuntime{}, wantID: srcID, wantSaves: 1, wantErr: "has no image"},
		{name: "ssh failure", saver: &fakeSaver{saveErr: errors.New("ssh login failed")}, rt: &fakeRuntime{}, wantID: srcID, wantSaves: 1, wantErr: "ssh login"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var last int64
			res, err := Transfer(context.Background(), c.saver, c.rt, Request{Ref: "app:1", WantID: c.wantID, MaxBytes: c.max, Progress: func(n int64) { last = n }})
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
			} else if err != nil {
				t.Fatalf("err = %v", err)
			}
			if c.saver.calls != c.wantSaves {
				t.Fatalf("saves = %d, want %d", c.saver.calls, c.wantSaves)
			}
			if res.Verified != c.wantVerify || res.AlreadyPresent != c.wantSkip {
				t.Fatalf("res = %+v", res)
			}
			if c.wantErr == "" && !c.wantSkip && (res.Bytes != int64(len(payload)) || last != res.Bytes) {
				t.Fatalf("bytes = %d, progress = %d", res.Bytes, last)
			}
			if c.max > 0 && res.Bytes > c.max+1 {
				t.Fatalf("read %d bytes past a %d limit", res.Bytes, c.max)
			}
		})
	}
}

func TestTransferRejectsBadInput(t *testing.T) {
	s := &fakeSaver{}
	if _, err := Transfer(context.Background(), s, &fakeRuntime{}, Request{Ref: "a;id"}); err == nil {
		t.Fatal("accepted a bad ref")
	}
	if _, err := Transfer(context.Background(), s, &fakeRuntime{}, Request{Ref: "a:1", WantID: "nope"}); err == nil {
		t.Fatal("accepted a bad image ID")
	}
	if s.calls != 0 {
		t.Fatal("saved despite invalid input")
	}
}

func TestTransferCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Transfer(ctx, &fakeSaver{data: []byte("x")}, &fakeRuntime{loadID: srcID}, Request{Ref: "a:1", WantID: srcID})
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("err = %v", err)
	}
}
