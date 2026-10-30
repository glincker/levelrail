package docker

import (
	"errors"
	"testing"
)

func TestCheckVolumeOwner(t *testing.T) {
	want := VolumeOwner{App: "web"}
	if err := checkVolumeOwner("v", map[string]string{VolumeOwnerAppLabel: "other"}, want); !errors.Is(err, ErrVolumeOwnerMismatch) {
		t.Errorf("different owner: err = %v, want mismatch", err)
	}
	if err := checkVolumeOwner("v", map[string]string{VolumeOwnerAppLabel: "web"}, want); err != nil {
		t.Errorf("same owner: err = %v", err)
	}
	if err := checkVolumeOwner("v", nil, want); err != nil {
		t.Errorf("unlabeled: err = %v, want accepted", err)
	}
	if err := checkVolumeOwner("v", map[string]string{VolumeOwnerAppLabel: "other"}, VolumeOwner{}); err != nil {
		t.Errorf("unknown owner: err = %v, want accepted", err)
	}
}
