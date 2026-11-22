package main

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

// A support bundle must never carry the caller's token or secret-named settings.
func TestWriteDoctorBundle_RedactsSecrets(t *testing.T) {
	settings := map[string]any{"settings": []any{
		map[string]any{"key": "smtp.password", "value": "hunter2", "default": ""},
		map[string]any{"key": "build.timeout", "value": 30},
	}, "api_token": "abc", "note": "bearer tok-123 seen"}
	var buf bytes.Buffer
	err := writeDoctorBundle(&buf, []bundleFile{{"settings.json", settings}}, []string{"nodes: boom"}, "tok-123", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		got[f.Name] = string(b)
	}
	s := got["settings.json"]
	for _, leak := range []string{"hunter2", "tok-123", `"abc"`} {
		if strings.Contains(s, leak) {
			t.Errorf("settings.json leaks %q: %s", leak, s)
		}
	}
	if !strings.Contains(s, `"value": 30`) || !strings.Contains(s, "build.timeout") {
		t.Errorf("non-secret setting lost: %s", s)
	}
	if !strings.Contains(got["errors.txt"], "nodes: boom") {
		t.Errorf("errors.txt = %q", got["errors.txt"])
	}
}
