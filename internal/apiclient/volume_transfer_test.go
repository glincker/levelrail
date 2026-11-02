package apiclient

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

func smallTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "a", Mode: 0o644, Size: 5, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("hello"))
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestCopyVerifiedArchive_AcceptsWholeArchive(t *testing.T) {
	full := smallTarGz(t)
	var out bytes.Buffer
	n, err := copyVerifiedArchive(&out, bytes.NewReader(full))
	if err != nil || n != int64(len(full)) || !bytes.Equal(out.Bytes(), full) {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestCopyVerifiedArchive_FlagsCutOffStreams(t *testing.T) {
	full := smallTarGz(t)
	cases := map[string]io.Reader{
		"cut in the middle": bytes.NewReader(full[:len(full)/2]),
		"missing trailer":   bytes.NewReader(full[:len(full)-4]),
		"empty":             strings.NewReader(""),
		"read error":        io.MultiReader(bytes.NewReader(full[:10]), errBody{}),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if _, err := copyVerifiedArchive(&out, body); !errors.Is(err, ErrExportTruncated) {
				t.Fatalf("err = %v, want export truncated", err)
			}
		})
	}
}

type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
