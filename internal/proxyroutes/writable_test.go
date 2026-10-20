package proxyroutes

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCheckWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	tests := []struct {
		name    string
		mode    os.FileMode
		wantErr string
	}{
		{"writable", 0o755, ""},
		{"read only for this user", 0o555, "may not create files"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			d, err := OpenDir(testNS, dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, tc.mode); err != nil { //nolint:gosec // test temp dir
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // restore so TempDir cleanup works
			err = d.CheckWritable()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var nw *NotWritableError
			if !errors.As(err, &nw) || !strings.Contains(err.Error(), tc.wantErr) || !strings.HasPrefix(err.Error(), NotWritableCode) {
				t.Fatalf("err = %v", err)
			}
			if werr := d.Write("acme-managed-a.example.com.yaml", []byte(testNS.Header()+"\n")); !errors.As(werr, &nw) {
				t.Errorf("Write err = %v", werr)
			}
		})
	}
}
