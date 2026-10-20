package main

import "testing"

func TestExportSpoolDir(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"under the data dir by default", map[string]string{"APP_DATA_DIR": "/srv/data"}, "/srv/data/export-spool"},
		{"explicit dir wins", map[string]string{"APP_DATA_DIR": "/srv/data", "APP_EXPORT_SPOOL_DIR": "/fast/spool"}, "/fast/spool"},
		{"can be turned off", map[string]string{"APP_DATA_DIR": "/srv/data", "APP_EXPORT_SPOOL": "false"}, ""},
	}
	for _, tt := range tests {
		if got := exportSpoolDir(env(tt.env)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
