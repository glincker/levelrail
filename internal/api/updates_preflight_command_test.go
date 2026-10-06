package api

import (
	"strings"
	"testing"
)

func TestUpgradeCommand(t *testing.T) {
	tests := []struct {
		name         string
		tag          string
		verify       bool
		want, reject string
	}{
		{"plain", "", false, "| sudo sh -s upgrade", "APP_INSTALL_VERIFY"},
		{"pinned tag", "v1.2.3", false, "sudo LEVELRAIL_VERSION=v1.2.3 sh -s upgrade", "APP_INSTALL_VERIFY"},
		{"verified", "", true, "sudo APP_INSTALL_VERIFY=require sh -s upgrade", "LEVELRAIL_VERSION"},
		{"pinned and verified", "v1.2.3", true, "sudo LEVELRAIL_VERSION=v1.2.3 APP_INSTALL_VERIFY=require sh -s upgrade", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := upgradeCommand(tc.tag, tc.verify)
			if !strings.Contains(got, tc.want) {
				t.Errorf("command = %q, want it to contain %q", got, tc.want)
			}
			if tc.reject != "" && strings.Contains(got, tc.reject) {
				t.Errorf("command = %q, must not contain %q", got, tc.reject)
			}
		})
	}
}
