package api

import (
	"testing"

	"github.com/glincker/levelrail/internal/giteaapp"
)

func TestGiteaCapabilitiesFromPermissions(t *testing.T) {
	caps := map[string]gitAccessCapability{}
	for _, c := range giteaCapabilities(giteaapp.Access{Sampled: 2, CanPull: true, CanPush: true}) {
		caps[c.ID] = c
	}
	if caps[capCommitStatus].State != gitCapAllowed || !caps[capCommitStatus].Verified {
		t.Errorf("commit status = %+v, want allowed and verified", caps[capCommitStatus])
	}
	if got := caps[capRegisterWebhook]; got.State != gitCapNeedsScope || got.NeedsScope == "" {
		t.Errorf("webhook = %+v, want needs_scope without admin", got)
	}
	if caps[capImportGroup].State != gitCapNeedsScope {
		t.Errorf("import = %+v, want needs_scope without organizations", caps[capImportGroup])
	}
}
