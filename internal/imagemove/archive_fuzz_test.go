package imagemove

import (
	"archive/tar"
	"bytes"
	"testing"
)

func FuzzScanArchive(f *testing.F) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := []byte(`{"config":"c.json"}`)
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
	_, _ = tw.Write(body)
	_ = tw.Close()
	f.Add(buf.Bytes(), "img:1")
	f.Add([]byte("garbage"), "")
	f.Fuzz(func(_ *testing.T, data []byte, ref string) {
		_, _ = scanArchive(bytes.NewReader(data), ref)
	})
}
