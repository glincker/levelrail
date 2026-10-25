package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdoptInstalledDataDir(t *testing.T) {
	cases := []struct {
		name string
		env  string
		unit string
		drop string
		want string
	}{
		{"env wins", "/set", "Environment=APP_DATA_DIR=/unit\n", "", ""},
		{"unit file", "", "[Service]\nEnvironment=APP_DATA_DIR=/srv/lr\n", "", "/srv/lr"},
		{"drop-in quoted", "", "", "[Service]\nEnvironment=\"APP_DATA_DIR=/srv/drop\" \"X=1\"\n", "/srv/drop"},
		{"none", "", "[Service]\nUser=x\n", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("APP_DATA_DIR", tc.env)
			if tc.unit != "" {
				mustWrite(t, filepath.Join(dir, systemdUnitName), tc.unit)
			}
			if tc.drop != "" {
				mustWrite(t, filepath.Join(dir, systemdUnitName+".d", "10-install.conf"), tc.drop)
			}
			got := adoptInstalledDataDir(dir)
			if got != tc.want {
				t.Errorf("adoptInstalledDataDir() = %q, want %q", got, tc.want)
			}
			if tc.want != "" && os.Getenv("APP_DATA_DIR") != tc.want {
				t.Errorf("APP_DATA_DIR = %q, want %q", os.Getenv("APP_DATA_DIR"), tc.want)
			}
		})
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
