package spec

import "testing"

func TestValidateBindMountHostPath(t *testing.T) {
	// validateBindMountHostPath delegates to bindmount.ValidateHostPath,
	// so we only do a minimal sanity check here to verify delegation
	// and avoid duplicating bindmount_test.go logic.

	if err := validateBindMountHostPath("/srv/myapp/data"); err != nil {
		t.Errorf("validateBindMountHostPath(/srv/myapp/data) expected no error, got %v", err)
	}

	if err := validateBindMountHostPath("/etc"); err == nil {
		t.Errorf("validateBindMountHostPath(/etc) expected error, got nil")
	}
}
