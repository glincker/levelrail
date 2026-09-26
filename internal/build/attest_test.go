package build

import (
	"archive/tar"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

type tarEntry struct {
	name string
	data string
	dir  bool
}

func buildTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.data)), Typeflag: tar.TypeReg}
		if e.dir {
			hdr = &tar.Header{Name: e.name, Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.data)); err != nil && !e.dir {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const (
	sbomStatement = `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://spdx.dev/Document","subject":[{"name":"_"}],"predicate":{"spdxVersion":"SPDX-2.3","packages":[]}}`
	cdxStatement  = `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://cyclonedx.org/bom","predicate":{"bomFormat":"CycloneDX"}}`
	provStatement = `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://slsa.dev/provenance/v0.2","predicate":{"builder":{"id":"x"}}}`
)

func sniff(t *testing.T, data []byte, chunk int) Attestations {
	t.Helper()
	s := newAttestSniffer(0)
	for len(data) > 0 {
		n := min(chunk, len(data))
		if w, err := s.Write(data[:n]); err != nil || w != n {
			t.Fatalf("Write = %d, %v", w, err)
		}
		data = data[n:]
	}
	return s.Finish()
}

func TestAttestSniffer_ExtractsPredicates(t *testing.T) {
	layout := buildTar(t, []tarEntry{
		{name: "oci-layout", data: `{"imageLayoutVersion":"1.0.0"}`},
		{name: "blobs/", dir: true},
		{name: "blobs/sha256/layer", data: strings.Repeat("x", 4096)},
		{name: "blobs/sha256/manifest", data: `{"schemaVersion":2}`},
		{name: "blobs/sha256/sbom", data: sbomStatement},
		{name: "blobs/sha256/prov", data: provStatement},
	})
	for _, chunk := range []int{1, 7, 512, 1 << 20} {
		got := sniff(t, layout, chunk)
		if !strings.Contains(string(got.SBOM), "spdxVersion") || strings.Contains(string(got.SBOM), "_type") {
			t.Errorf("chunk %d: SBOM must be the bare predicate, got %q", chunk, got.SBOM)
		}
		if !strings.Contains(string(got.Provenance), "builder") {
			t.Errorf("chunk %d: provenance = %q", chunk, got.Provenance)
		}
	}
}

func TestAttestSniffer_CycloneDX(t *testing.T) {
	got := sniff(t, buildTar(t, []tarEntry{{name: "blobs/sha256/a", data: cdxStatement}}), 64)
	if !strings.Contains(string(got.SBOM), "CycloneDX") {
		t.Errorf("SBOM = %q", got.SBOM)
	}
}

func TestAttestSniffer_NothingToFind(t *testing.T) {
	tests := map[string][]byte{
		"plain image":            buildTar(t, []tarEntry{{name: "blobs/sha256/layer", data: "layer bytes"}, {name: "manifest.json", data: sbomStatement}}),
		"marker but not a json":  buildTar(t, []tarEntry{{name: "blobs/sha256/a", data: "in-toto.io/Statement garbage"}}),
		"statement without body": buildTar(t, []tarEntry{{name: "blobs/sha256/a", data: `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://spdx.dev/Document"}`}}),
		"unknown predicate type": buildTar(t, []tarEntry{{name: "blobs/sha256/a", data: `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://example.com/x","predicate":{}}`}}),
		"not a tar at all":       []byte(strings.Repeat("not a tar ", 1000)),
		"truncated tar":          buildTar(t, []tarEntry{{name: "blobs/sha256/a", data: sbomStatement}})[:100],
		"empty stream":           nil,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			got := sniff(t, data, 100)
			if len(got.SBOM) != 0 || len(got.Provenance) != 0 {
				t.Errorf("found %+v", got)
			}
		})
	}
}

func TestAttestSniffer_OversizedBlobIsSkipped(t *testing.T) {
	s := newAttestSniffer(64)
	_, _ = s.Write(buildTar(t, []tarEntry{{name: "blobs/sha256/a", data: sbomStatement}}))
	if got := s.Finish(); len(got.SBOM) != 0 {
		t.Errorf("a blob over the limit must be skipped, got %q", got.SBOM)
	}
}

func TestAttestSniffer_NeverBlocksTheImageStream(t *testing.T) {
	junk := bytes.Repeat([]byte("z"), 8<<20)
	s := newAttestSniffer(0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := io.Copy(io.MultiWriter(io.Discard, s), bytes.NewReader(junk)); err != nil {
			t.Errorf("copy: %v", err)
		}
		s.Finish()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the sniffer stalled the image stream")
	}
}

func TestNewSolveOpt_AttestAttrs(t *testing.T) {
	for _, attest := range []bool{false, true} {
		opt, err := newSolveOpt(Request{ContextDir: "testdata", Tag: "t:1", Attest: attest}, CacheConfig{}, discardWriteCloser{io.Discard})
		if err != nil {
			t.Fatal(err)
		}
		sbom, hasSBOM := opt.FrontendAttrs["attest:sbom"]
		prov := opt.FrontendAttrs["attest:provenance"]
		if hasSBOM != attest || sbom != "" || (prov == "mode=min") != attest {
			t.Errorf("attest=%v: attrs = %v", attest, opt.FrontendAttrs)
		}
	}
}
