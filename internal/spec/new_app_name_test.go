package spec

import "testing"

func TestValidateNewAppNameRefusesPreviewTail(t *testing.T) {
	if err := ValidateNewAppName("web-pr-1-x"); err != nil {
		t.Fatalf("name with pr in the middle must pass: %v", err)
	}
	if err := ValidateNewAppName("web-pr-12"); err == nil {
		t.Fatal("name ending in -pr-<digits> must be refused")
	}
}
