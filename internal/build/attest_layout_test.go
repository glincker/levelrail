package build

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func blobName(data string) string {
	sum := sha256.Sum256([]byte(data))
	return "blobs/sha256/" + hex.EncodeToString(sum[:])
}

func gzipped(t *testing.T, payload string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// exporterArchive mirrors the OCI layout the BuildKit docker exporter streams for an
// image with attestations: an index pointing at an image manifest and an attestation
// manifest whose layers are content-addressed in-toto statements, next to the image
// config and gzipped layers. Statements are inserted in the given order among the
// image blobs.
func exporterArchive(t *testing.T, prefix string, statements ...string) []byte {
	t.Helper()
	layer := gzipped(t, strings.Repeat("filesystem bytes ", 2048))
	config := `{"architecture":"amd64","os":"linux","rootfs":{"type":"layers","diff_ids":["sha256:aa"]}}`
	imgManifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:%s"},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"sha256:%s"}]}`,
		strings.TrimPrefix(blobName(config), "blobs/sha256/"), strings.TrimPrefix(blobName(layer), "blobs/sha256/"))
	attManifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","layers":[{"mediaType":"application/vnd.in-toto+json","annotations":{"in-toto.io/predicate-type":"https://spdx.dev/Document"}}]}`
	index := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"annotations":{"vnd.docker.reference.type":"attestation-manifest"}}]}`

	entries := []tarEntry{
		{name: prefix + "oci-layout", data: `{"imageLayoutVersion":"1.0.0"}`},
		{name: prefix + "index.json", data: index},
		{name: prefix + "manifest.json", data: `[{"Config":"blobs/sha256/x","RepoTags":["app:1"],"Layers":["blobs/sha256/y"]}]`},
		{name: prefix + "blobs/", dir: true},
		{name: prefix + "blobs/sha256/", dir: true},
		{name: prefix + blobName(config), data: config},
		{name: prefix + blobName(layer), data: layer},
		{name: prefix + blobName(imgManifest), data: imgManifest},
		{name: prefix + blobName(attManifest), data: attManifest},
	}
	for _, st := range statements {
		entries = append(entries, tarEntry{name: prefix + blobName(st), data: st})
	}
	return buildTar(t, entries)
}

func longSubject() string {
	var b strings.Builder
	for i := range 20 {
		fmt.Fprintf(&b, `{"name":"pkg:oci/app-%d","digest":{"sha256":"%064d"}},`, i, i)
	}
	return strings.TrimSuffix(b.String(), ",")
}

func TestAttestSniffer_DockerExporterLayout(t *testing.T) {
	sbomV01 := `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://spdx.dev/Document","subject":[{"name":"_","digest":{"sha256":"` + strings.Repeat("a", 64) + `"}}],"predicate":{"spdxVersion":"SPDX-2.3","name":"sbom","packages":[{"name":"openssl"}]}}`
	provV02 := `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://slsa.dev/provenance/v0.2","subject":[{"name":"_","digest":{"sha256":"` + strings.Repeat("a", 64) + `"}}],"predicate":{"builder":{"id":""},"buildType":"https://mobyproject.org/buildkit@v1"}}`
	sbomV1 := `{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://spdx.dev/Document","subject":[{"name":"_","digest":{"sha256":"` + strings.Repeat("b", 64) + `"}}],"predicate":{"spdxVersion":"SPDX-2.3","name":"v1"}}`
	provV1 := `{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://slsa.dev/provenance/v1","subject":[],"predicate":{"buildDefinition":{"buildType":"x"}}}`
	longSBOM := `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://spdx.dev/Document","subject":[` + longSubject() + `],"predicate":{"spdxVersion":"SPDX-2.3","name":"long"}}`

	tests := []struct {
		name       string
		prefix     string
		statements []string
		wantSBOM   string
		wantProv   string
	}{
		{name: "v0.1 statements", statements: []string{sbomV01, provV02}, wantSBOM: "openssl", wantProv: "mobyproject"},
		{name: "provenance before sbom", statements: []string{provV02, sbomV01}, wantSBOM: "openssl", wantProv: "mobyproject"},
		{name: "v1 statements", statements: []string{sbomV1, provV1}, wantSBOM: `"name":"v1"`, wantProv: "buildDefinition"},
		{name: "subject list longer than the sniff window", statements: []string{longSBOM}, wantSBOM: `"name":"long"`},
		{name: "dot slash prefixed entries", prefix: "./", statements: []string{sbomV01, provV02}, wantSBOM: "openssl", wantProv: "mobyproject"},
		{name: "sbom only", statements: []string{sbomV01}, wantSBOM: "openssl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			archive := exporterArchive(t, tc.prefix, tc.statements...)
			for _, chunk := range []int{3, 512, 32 << 10} {
				got := sniff(t, archive, chunk)
				if tc.wantSBOM == "" != (len(got.SBOM) == 0) || !strings.Contains(string(got.SBOM), tc.wantSBOM) || strings.Contains(string(got.SBOM), "_type") {
					t.Errorf("chunk %d: SBOM = %.120q, want bare predicate containing %q", chunk, got.SBOM, tc.wantSBOM)
				}
				if !strings.Contains(string(got.Provenance), tc.wantProv) || (tc.wantProv == "" && len(got.Provenance) != 0) {
					t.Errorf("chunk %d: provenance = %.120q, want %q", chunk, got.Provenance, tc.wantProv)
				}
			}
		})
	}
}

func TestAttestSniffer_DockerExporterLayoutWithoutAttestations(t *testing.T) {
	got := sniff(t, exporterArchive(t, ""), 4096)
	if len(got.SBOM) != 0 || len(got.Provenance) != 0 {
		t.Errorf("an image without attestations must yield none, got %+v", got)
	}
}

func TestAttestSniffer_ClassicDockerSaveLayoutYieldsNothing(t *testing.T) {
	archive := buildTar(t, []tarEntry{
		{name: "manifest.json", data: `[{"Config":"abc.json","RepoTags":["app:1"],"Layers":["def/layer.tar"]}]`},
		{name: "abc.json", data: `{"architecture":"amd64"}`},
		{name: "def/", dir: true},
		{name: "def/layer.tar", data: strings.Repeat("l", 2048)},
	})
	if got := sniff(t, archive, 512); len(got.SBOM) != 0 || len(got.Provenance) != 0 {
		t.Errorf("a layout without blobs/ carries no attestations, got %+v", got)
	}
}
