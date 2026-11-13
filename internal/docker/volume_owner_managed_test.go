package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestDirWritableBy(t *testing.T) {
	tests := []struct {
		name             string
		uid, gid         int
		ownerUID, ownGID int
		mode             int64
		want             bool
	}{
		{"root writes anywhere", 0, 0, 1000, 1000, 0o555, true},
		{"owner with rwx", 1001, 1001, 1001, 1001, 0o755, true},
		{"owner without write", 1001, 1001, 1001, 1001, 0o555, false},
		{"group with rwx", 1001, 50, 0, 50, 0o775, true},
		{"group without write", 1001, 50, 0, 50, 0o755, false},
		{"other with rwx", 1001, 1001, 0, 0, 0o777, true},
		{"root-owned 755", 1001, 1001, 0, 0, 0o755, false},
		{"other write without search", 1001, 1001, 0, 0, 0o772, false},
	}
	for _, tt := range tests {
		if got := dirWritableBy(tt.uid, tt.gid, tt.ownerUID, tt.ownGID, tt.mode); got != tt.want {
			t.Errorf("%s: dirWritableBy = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRootChownWanted(t *testing.T) {
	rootDir := &tar.Header{Typeflag: tar.TypeDir, Mode: 0o755}
	tests := []struct {
		name           string
		root           *tar.Header
		empty, managed bool
		want           bool
	}{
		{"fresh root-owned", rootDir, true, false, true},
		{"filled, not created by us", rootDir, false, false, false},
		{"filled, created by us, unwritable", rootDir, false, true, true},
		{"filled, created by us, world writable", &tar.Header{Typeflag: tar.TypeDir, Mode: 0o777}, false, true, false},
		{"owned by another user", &tar.Header{Typeflag: tar.TypeDir, Uid: 1000, Gid: 1000, Mode: 0o755}, false, true, false},
		{"not a directory", &tar.Header{Typeflag: tar.TypeReg}, true, true, false},
		{"no header", nil, true, true, false},
	}
	for _, tt := range tests {
		if got := rootChownWanted(tt.root, tt.empty, tt.managed, 1001, 1001); got != tt.want {
			t.Errorf("%s: rootChownWanted = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// fakeVolumeDaemon serves the calls the chown and probe paths make; dirs maps a
// container path to its tar stream and volumeLabels is what volume inspect returns.
type fakeVolumeDaemon struct {
	imageUser, containerUser string
	users                    string
	dirs                     map[string][]byte
	volumeLabels             string

	mu       sync.Mutex
	chownCmd string
}

func tarOf(t *testing.T, hdrs ...*tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range hdrs {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if h.Size > 0 {
			_, _ = tw.Write(make([]byte, h.Size))
		}
	}
	_ = tw.Close()
	return buf.Bytes()
}

func passwdTar(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "passwd", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	return buf.Bytes()
}

func (f *fakeVolumeDaemon) serve(t *testing.T) *Client {
	t.Helper()
	stat := base64.StdEncoding.EncodeToString([]byte(`{"name":"x","size":0,"mode":2147484141}`))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(p, "/_ping"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(p, "/images/") && strings.HasSuffix(p, "/json"):
			_, _ = w.Write([]byte(`{"Id":"sha256:1","Config":{"User":"` + f.imageUser + `"}}`))
		case strings.HasSuffix(p, "/containers/cid/json"):
			_, _ = w.Write([]byte(`{"Id":"cid","Config":{"User":"` + f.containerUser + `"}}`))
		case strings.HasSuffix(p, "/containers/cid/archive"):
			path := r.URL.Query().Get("path")
			body, ok := f.dirs[path]
			if path == "/etc/passwd" {
				body, ok = passwdTar(t, f.users), true
			}
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("X-Docker-Container-Path-Stat", stat)
			w.Header().Set("Content-Type", "application/x-tar")
			_, _ = w.Write(body)
		case strings.Contains(p, "/volumes/"):
			_, _ = w.Write([]byte(`{"Name":"v","Labels":` + f.volumeLabels + `}`))
		case strings.HasSuffix(p, "/containers/create"):
			var body bytes.Buffer
			_, _ = body.ReadFrom(r.Body)
			f.mu.Lock()
			f.chownCmd = body.String()
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"Id":"helper"}`))
		case strings.HasSuffix(p, "/containers/helper/start"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(p, "/containers/helper/wait"):
			_, _ = w.Write([]byte(`{"StatusCode":0}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func (f *fakeVolumeDaemon) chowned() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chownCmd
}

// A Levelrail volume that already holds root-owned files (an earlier deploy as
// root) gets its root handed to the named image user; an unlabelled one does not.
func TestChownFreshVolumes_ManagedFilledRootOwnedVolume(t *testing.T) {
	filled := func(t *testing.T) []byte {
		return tarOf(t,
			&tar.Header{Name: "cache/", Typeflag: tar.TypeDir, Mode: 0o755},
			&tar.Header{Name: "cache/images/", Typeflag: tar.TypeDir, Mode: 0o755})
	}
	for _, tt := range []struct {
		name, labels string
		want         bool
	}{
		{"created by levelrail", `{"` + VolumeOwnerAppLabel + `":"web"}`, true},
		{"not created by levelrail", `null`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeVolumeDaemon{
				imageUser: "nextjs", users: "root:x:0:0::/root:/bin/sh\nnextjs:x:1001:1001::/app:/sbin/nologin\n",
				dirs: map[string][]byte{"/app/.next/cache": filled(t)}, volumeLabels: tt.labels,
			}
			c := f.serve(t)
			vols := []VolumeMount{{Name: "app-web-cache", ContainerPath: "/app/.next/cache"}}
			if err := c.chownFreshVolumes(context.Background(), "cid", "img:1", "", vols); err != nil {
				t.Fatalf("chownFreshVolumes() error = %v", err)
			}
			got := strings.Contains(f.chowned(), `"1001:1001"`)
			if got != tt.want {
				t.Errorf("chowned to 1001:1001 = %v, want %v (create body %q)", got, tt.want, f.chowned())
			}
		})
	}
}
