package api

import (
	"strings"
	"testing"
)

func TestUpgradeCommand(t *testing.T) {
	tests := []struct {
		name         string
		tag          string
		want, reject string
	}{
		{"plain", "", "| sudo sh -s upgrade", "APP_INSTALL_VERIFY"},
		{"pinned tag", "v1.2.3", "sudo LEVELRAIL_VERSION=v1.2.3 sh -s upgrade", "APP_INSTALL_VERIFY"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := upgradeCommand(tc.tag)
			if !strings.Contains(got, tc.want) {
				t.Errorf("command = %q, want it to contain %q", got, tc.want)
			}
			if tc.reject != "" && strings.Contains(got, tc.reject) {
				t.Errorf("command = %q, must not contain %q", got, tc.reject)
			}
		})
	}
}
