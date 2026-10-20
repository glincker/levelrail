package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestConfirmDatabaseName(t *testing.T) {
	tests := []struct {
		name, flag, stdin string
		wantErr           bool
	}{
		{"main", "main", "", false},
		{"main", "other", "", true},
		{"main", "", "main\n", false},
		{"main", "", "nope\n", true},
		{"main", "", "", true},
	}
	for _, tt := range tests {
		err := confirmDatabaseName(tt.name, tt.flag, "warn", strings.NewReader(tt.stdin), &bytes.Buffer{})
		if (err != nil) != tt.wantErr {
			t.Errorf("confirmDatabaseName(%q, %q, stdin %q) error = %v, wantErr %v", tt.name, tt.flag, tt.stdin, err, tt.wantErr)
		}
	}
}

func TestPrintMajorUpgrades(t *testing.T) {
	var buf bytes.Buffer
	printMajorUpgrades(&buf, []apiclient.MajorUpgradeResource{
		{ID: "mu_1", FromVersion: "16", ToVersion: "17", Status: "succeeded", Phase: "verify", SnapshotVolume: "db-main-data-pre16-mu1"},
		{ID: "mu_2", FromVersion: "16", ToVersion: "17", Status: "rolled_back", Error: "boom"},
	})
	out := buf.String()
	for _, want := range []string{"mu_1", "db-main-data-pre16-mu1", "rolled_back", "boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
