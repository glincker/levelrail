package imagemove

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	layers  []string
	loadErr error
	loaded  []byte
}

func (f *fakeRuntime) LoadImage(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	f.loaded = b
	if err != nil {
		return err
	}
	return f.loadErr
}

func (f *fakeRuntime) InspectImageID(_ context.Context, ref string) (string, error) {
	if f.loaded != nil && f.loadErr == nil && f.loadID != "" {
		return f.loadID, nil
	}
	return f.images[ref], nil
}

func (f *fakeRuntime) InspectImageLayers(_ context.Context, _ string) ([]string, error) {
	if f.loaded != nil && f.loadErr == nil {
		return f.layers, nil
	}
	return nil, nil
}

type tarEntry struct {
	name string
	body []byte
	typ  byte
}

func buildTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o600, Size: int64(len(e.body)), Typeflag: typ}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

var testLayers = []string{
	"sha256:aaaa000000000000000000000000000000000000000000000000000000000001",
	"sha256:aaaa000000000000000000000000000000000000000000000000000000000002",
}

func testConfig(t *testing.T, layers []string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"architecture": "amd64", "rootfs": map[string]any{"type": "layers", "diff_ids": layers}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func dockerArchive(t *testing.T, cfg []byte, extra ...tarEntry) []byte {
	t.Helper()
	cfgName := strings.TrimPrefix(digestOf(cfg), "sha256:") + ".json"
	manifest := []byte(`[{"Config":"` + cfgName + `","RepoTags":["app:1"],"Layers":["l1/layer.tar","l2/layer.tar"]}]`)
	entries := append([]tarEntry{{name: cfgName, body: cfg}, {name: "l1/layer.tar", body: []byte("layer-one")}}, extra...)
	return buildTar(t, append(entries, tarEntry{name: "manifest.json", body: manifest}))
}

func ociArchive(t *testing.T, cfg []byte) []byte {
	t.Helper()
	cfgD := digestOf(cfg)
	man := []byte(`{"schemaVersion":2,"config":{"digest":"` + cfgD + `"},"layers":[]}`)
	manD := digestOf(man)
	idx := []byte(`{"schemaVersion":2,"manifests":[{"digest":"` + manD + `"}]}`)
	return buildTar(t, []tarEntry{
		{name: "oci-layout", body: []byte(`{"imageLayoutVersion":"1.0.0"}`)},
		{name: "blobs/sha256/" + strings.TrimPrefix(cfgD, "sha256:"), body: cfg},
		{name: "blobs/sha256/" + strings.TrimPrefix(manD, "sha256:"), body: man},
		{name: "index.json", body: idx},
	})
}

func TestTransfer(t *testing.T) {
	cfg := testConfig(t, testLayers)
	cfgDigest := digestOf(cfg)
	docker := dockerArchive(t, cfg)
	pad := tarEntry{name: "big/layer.tar", body: bytes.Repeat([]byte{0}, 8192)}
	padded := dockerArchive(t, cfg, pad)
	otherLayers := []string{testLayers[0], "sha256:bbbb000000000000000000000000000000000000000000000000000000000009"}
	noConfig := buildTar(t, []tarEntry{{name: "manifest.json", body: []byte(`[{"Config":"gone.json"}]`)}})
	traversal := dockerArchive(t, cfg,
		tarEntry{name: "../../etc/manifest.json", body: []byte(`[{"Config":"evil.json"}]`)},
		tarEntry{name: "/abs/index.json", body: []byte(`{"manifests":[]}`)},
		tarEntry{name: strings.Repeat("a/", 2000) + "x.json", body: []byte(`{}`)},
		tarEntry{name: "link", typ: tar.TypeSymlink},
	)
	hugeEntry := dockerArchive(t, cfg, tarEntry{name: "huge.json", body: append([]byte("{"), bytes.Repeat([]byte(" "), maxJSONEntryBytes+10)...)})

	cases := []struct {
		name       string
		saver      *fakeSaver
		rt         *fakeRuntime
		wantID     string
		max        int64
		wantErr    []string
		wantSaves  int
		wantVerify bool
		wantSkip   bool
		wantDigest string
	}{
		{name: "classic source id equals config digest", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: cfgDigest, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "containerd target id differs but layers equal", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{loadID: otherID, layers: testLayers}, wantID: cfgDigest, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "same store loaded id equals source id", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "oci layout archive", saver: &fakeSaver{data: ociArchive(t, cfg)}, rt: &fakeRuntime{loadID: otherID, layers: testLayers}, wantID: cfgDigest, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "no source id needs only the layer check", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{loadID: otherID, layers: testLayers}, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "already present skips the stream", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{images: map[string]string{"app:1": srcID}}, wantID: srcID, wantVerify: true, wantSkip: true},
		{name: "stale tag on target is replaced", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{images: map[string]string{"app:1": otherID}, loadID: srcID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "layers differ", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{loadID: srcID, layers: otherLayers}, wantID: srcID, wantSaves: 1, wantErr: []string{"layers that differ"}, wantDigest: cfgDigest},
		{name: "snapshot id matches neither", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{loadID: otherID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantErr: []string{"neither", srcID, cfgDigest, otherID}, wantDigest: cfgDigest},
		{name: "config blob missing", saver: &fakeSaver{data: noConfig}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantErr: []string{"no readable config"}},
		{name: "truncated archive", saver: &fakeSaver{data: docker[:len(docker)/2]}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantErr: []string{"verify the image content"}},
		{name: "not a tar", saver: &fakeSaver{data: []byte(strings.Repeat("x", 4096))}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantErr: []string{"verify the image content"}},
		{name: "hostile entry names are ignored", saver: &fakeSaver{data: traversal}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "oversized json entry is skipped", saver: &fakeSaver{data: hugeEntry}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: srcID, wantSaves: 1, wantVerify: true, wantDigest: cfgDigest},
		{name: "remote failure surfaces stderr", saver: &fakeSaver{closeErr: errors.New("docker save on the source failed: No such image")}, rt: &fakeRuntime{loadErr: errors.New("empty archive")}, wantID: srcID, wantSaves: 1, wantErr: []string{"No such image"}},
		{name: "load failure", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{loadErr: errors.New("disk full")}, wantID: srcID, wantSaves: 1, wantErr: []string{"disk full"}},
		{name: "size bound", saver: &fakeSaver{data: padded}, rt: &fakeRuntime{loadID: srcID, layers: testLayers}, wantID: srcID, max: 1000, wantSaves: 1, wantErr: []string{"size limit"}},
		{name: "image missing after load", saver: &fakeSaver{data: docker}, rt: &fakeRuntime{}, wantID: srcID, wantSaves: 1, wantErr: []string{"has no image"}},
		{name: "ssh failure", saver: &fakeSaver{saveErr: errors.New("ssh login failed")}, rt: &fakeRuntime{}, wantID: srcID, wantSaves: 1, wantErr: []string{"ssh login"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var last int64
			res, err := Transfer(context.Background(), c.saver, c.rt, Request{Ref: "app:1", WantID: c.wantID, MaxBytes: c.max, Progress: func(n int64) { last = n }})
			if len(c.wantErr) > 0 {
				if err == nil {
					t.Fatalf("err = nil, want %v", c.wantErr)
				}
				for _, w := range c.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Fatalf("err = %v, want it to contain %q", err, w)
					}
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
			if c.wantDigest != "" && res.ConfigDigest != c.wantDigest {
				t.Fatalf("config digest = %q, want %q", res.ConfigDigest, c.wantDigest)
			}
			if len(c.wantErr) == 0 && !c.wantSkip && (res.Bytes != int64(len(c.saver.data)) || last != res.Bytes) {
				t.Fatalf("bytes = %d, progress = %d, want %d", res.Bytes, last, len(c.saver.data))
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
