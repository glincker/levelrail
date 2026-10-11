package backup

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func buildTar(t *testing.T, entries []tar.Header, bodies map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range entries {
		h := h
		body := bodies[h.Name]
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(body))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte(body))
		}
	}
	_ = tw.Close()
	return buf.Bytes()
}

func TestComputeManifest(t *testing.T) {
	a := tar.Header{Name: "./a.txt", Mode: 0o644, Typeflag: tar.TypeReg}
	b := tar.Header{Name: "./dir/b.txt", Mode: 0o600, Uid: 1000, Typeflag: tar.TypeReg}
	d := tar.Header{Name: "./dir/", Mode: 0o750, Typeflag: tar.TypeDir}
	root := tar.Header{Name: "./", Mode: 0o755, Typeflag: tar.TypeDir}
	ln := tar.Header{Name: "./ln", Typeflag: tar.TypeSymlink, Linkname: "a.txt"}
	bodies := map[string]string{"./a.txt": "alpha", "./dir/b.txt": "bravo"}

	base := buildTar(t, []tar.Header{root, a, d, b, ln}, bodies)
	m, err := ComputeManifest(bytes.NewReader(base))
	if err != nil {
		t.Fatal(err)
	}
	if m.Files != 3 || m.Dirs != 1 || m.Bytes != 10 {
		t.Fatalf("manifest = %+v", m)
	}

	cases := []struct {
		name   string
		tar    []byte
		wantEq bool
	}{
		{"different order and no root entry", buildTar(t, []tar.Header{ln, b, d, a}, bodies), true},
		{"changed content", buildTar(t, []tar.Header{a, d, b, ln}, map[string]string{"./a.txt": "alphA", "./dir/b.txt": "bravo"}), false},
		{"changed owner", buildTar(t, []tar.Header{a, d, func() tar.Header { x := b; x.Uid = 0; return x }(), ln}, bodies), false},
		{"changed mode", buildTar(t, []tar.Header{func() tar.Header { x := a; x.Mode = 0o600; return x }(), d, b, ln}, bodies), false},
		{"missing file", buildTar(t, []tar.Header{a, d, ln}, bodies), false},
		{"changed symlink target", buildTar(t, []tar.Header{a, d, b, tar.Header{Name: "./ln", Typeflag: tar.TypeSymlink, Linkname: "elsewhere"}}, bodies), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ComputeManifest(bytes.NewReader(c.tar))
			if err != nil {
				t.Fatal(err)
			}
			if (got.Digest == m.Digest) != c.wantEq {
				t.Fatalf("digest equal = %v, want %v", got.Digest == m.Digest, c.wantEq)
			}
		})
	}

	if _, err := ComputeManifest(bytes.NewReader(base[:700])); err == nil {
		t.Fatal("a truncated tar must be an error")
	}
}

func TestCheckObject(t *testing.T) {
	h := store.BackupHistory{ObjectKey: "k", SizeBytes: 100}
	cases := []struct {
		name  string
		info  ObjectInfo
		hist  store.BackupHistory
		state string
	}{
		{"intact", ObjectInfo{Exists: true, Size: 100}, h, ObjectOK},
		{"missing", ObjectInfo{}, h, ObjectMissing},
		{"truncated", ObjectInfo{Exists: true, Size: 40}, h, ObjectTruncated},
		{"grown legacy raw object", ObjectInfo{Exists: true, Size: 140}, h, ObjectTruncated},
		{"legacy row without size only needs to exist", ObjectInfo{Exists: true, Size: 7}, store.BackupHistory{ObjectKey: "k"}, ObjectOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state, reason := CheckObject(c.info, c.hist)
			if state != c.state || (state != ObjectOK && reason == "") {
				t.Fatalf("state = %q reason = %q", state, reason)
			}
		})
	}
}

func TestBucketProtection_LevelAndWarning(t *testing.T) {
	cases := []struct {
		name  string
		p     BucketProtection
		level string
		warn  string
	}{
		{"locked", BucketProtection{ObjectLock: true, CanDelete: true}, ProtectionLocked, ""},
		{"versioned", BucketProtection{Versioning: "Enabled", CanDelete: true}, ProtectionVersion, "object lock is off"},
		{"open and deletable", BucketProtection{CanDelete: true}, ProtectionOpen, "can delete or overwrite"},
		{"suspended versioning is open", BucketProtection{Versioning: "Suspended", CanDelete: true}, ProtectionOpen, "can delete or overwrite"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.p.Level() != c.level {
				t.Fatalf("level = %q", c.p.Level())
			}
			if w := c.p.Warning(); !strings.Contains(w, c.warn) || (c.warn == "") != (w == "") {
				t.Fatalf("warning = %q", w)
			}
		})
	}
}
