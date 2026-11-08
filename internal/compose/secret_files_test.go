package compose

import (
	"strings"
	"testing"
)

const secretCompose = //nolint:gosec // test fixture, not a credential
`services:
  app:
    image: nginx:1.27
    secrets:
      - source: tls
        target: key.pem
secrets:
  tls:
    content: |
      -----BEGIN KEY-----
      pa$$word ${NOPE}
`

func TestInlineSecretsBecomeVerbatimSecretFiles(t *testing.T) {
	f, err := Parse([]byte(secretCompose))
	if err != nil {
		t.Fatal(err)
	}
	ds, _, err := ToDesiredServices("a", f)
	if err != nil {
		t.Fatal(err)
	}
	got := ds[0].Files
	if len(got) != 1 || got[0].Path != "/run/secrets/key.pem" || got[0].Mode != "0400" || !got[0].Secret || !got[0].NoExpand {
		t.Fatalf("files = %+v", got)
	}
	if !strings.Contains(got[0].Content, "${NOPE}") || !strings.Contains(got[0].Content, "pa$$word") {
		t.Fatalf("content altered: %q", got[0].Content)
	}
}

func TestRedactSecretContentKeepsParseable(t *testing.T) {
	out, changed, err := RedactSecretContent([]byte(secretCompose))
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if strings.Contains(string(out), "BEGIN KEY") || !strings.Contains(string(out), RedactedSecretMarker) {
		t.Fatalf("not redacted: %s", out)
	}
	f, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	ds, _, _ := ToDesiredServices("a", f)
	if ds[0].Files[0].Content != RedactedSecretMarker {
		t.Fatalf("marker lost: %+v", ds[0].Files)
	}
	again, changed, _ := RedactSecretContent(out)
	if changed || string(again) != string(out) {
		t.Fatal("redaction is not idempotent")
	}
}
