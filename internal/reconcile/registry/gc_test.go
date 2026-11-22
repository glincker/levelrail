package registry

import "testing"

func TestParseGCDryRun(t *testing.T) {
	out := `INFO[0000] Deleting blob: x
blob eligible for deletion: sha256:aa11bb22
time="x" level=info msg="x"
blob eligible for deletion: sha256:cc33dd44
`
	got := ParseGCDryRun(out)
	if len(got) != 2 || got[0] != "sha256:aa11bb22" {
		t.Fatalf("ParseGCDryRun = %v", got)
	}
}
