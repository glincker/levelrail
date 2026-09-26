package docker

import (
	"archive/tar"
	"errors"
	"io"
	"testing"
)

func TestSingleFileTar(t *testing.T) {
	r, err := singleFileTar("/tmp/sbom.json", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(r)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != "tmp/sbom.json" || hdr.Size != 7 {
		t.Errorf("header = %+v", hdr)
	}
	body, _ := io.ReadAll(tr)
	if string(body) != `{"a":1}` {
		t.Errorf("body = %q", body)
	}
	if _, err := tr.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("expected a single entry, got %v", err)
	}
}
