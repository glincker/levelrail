package plugin

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleDir = "testdata/marketplace"

// regenerateSample rewrites the checked-in sample with fresh throwaway keys.
// Only the public halves are kept, so nothing here can sign for a real index.
func regenerateSample(t *testing.T) {
	t.Helper()
	manifest, err := os.ReadFile("../../test/fixtures/plugin-notify/plugin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	man, err := ParseManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rootPub, rootPriv, _ := ed25519.GenerateKey(rand.Reader)
	pubPub, pubPriv, _ := ed25519.GenerateKey(rand.Reader)
	entry, err := BuildEntry(manifest, "https://plugins.example.com/example-notify/plugin.yaml", Sign(pubPriv, manifest, man.Digest()), pubPub,
		Entry{Name: "Example notify", Categories: []string{"sample"}, Changelog: "Initial sample listing."})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignIndex(rootPriv, Index{
		Version: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Sequence: 1,
		PublisherKeys: []IndexKey{{KeyID: Fingerprint(pubPub), Name: "Example Org", PublicKey: b64(pubPub)}},
		Plugins:       []Entry{entry},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sampleDir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"index.json":    append(signed, '\n'),
		"plugin.yaml":   manifest,
		"root.pub":      []byte(b64(rootPub) + "\n"),
		"publisher.pub": []byte(b64(pubPub) + "\n"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(sampleDir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSampleIndexVerifiesUnderItsThrowawayRoot(t *testing.T) {
	if os.Getenv("UPDATE_MARKETPLACE_SAMPLE") == "1" {
		regenerateSample(t)
	}
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(sampleDir, name)) //nolint:gosec // fixed testdata directory
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	root, err := ParsePublicKey(string(read("root.pub")))
	if err != nil {
		t.Fatal(err)
	}
	idx, _, err := ParseIndex(read("index.json"), root)
	if err != nil {
		t.Fatalf("sample index does not verify: %v", err)
	}
	if len(idx.Plugins) != 1 || idx.Plugins[0].ID != "example-notify" {
		t.Fatalf("sample plugins = %+v", idx.Plugins)
	}
	e := idx.Plugins[0]
	pk := TrustedKey{KeyID: e.PublisherKeyID, PublicKey: strings.TrimSpace(string(read("publisher.pub")))}
	if err := Verify([]TrustedKey{pk}, pk.KeyID, read("plugin.yaml"), e.Digest, e.ManifestSignature); err != nil {
		t.Fatalf("sample manifest signature: %v", err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, _, err := ParseIndex(read("index.json"), other); err == nil {
		t.Fatal("the sample verified under an unrelated key")
	}
}
