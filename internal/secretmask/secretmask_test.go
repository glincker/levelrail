package secretmask

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	r := New()
	r.Add("short")
	r.Add("supersecretvalue1")
	r.Add("supersecret")
	r.Add("supersecretvalue1-extended")
	for _, tc := range []struct{ in, want string }{
		{"token=supersecretvalue1 ok", "token=[redacted] ok"},
		{"x supersecretvalue1-extended y", "x [redacted] y"},
		{"short stays", "short stays"},
		{"", ""},
		{"nothing here", "nothing here"},
	} {
		if got := r.Mask(tc.in); got != tc.want {
			t.Errorf("Mask(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if r.Len() != 3 {
		t.Errorf("Len = %d, want 3 (short value ignored)", r.Len())
	}
}

func TestHandlerMasksMessageAttrsGroupsAndErrors(t *testing.T) {
	Add("hunter2-hunter2")
	var buf bytes.Buffer
	log := slog.New(Handler(slog.NewTextHandler(&buf, nil))).With(slog.String("pre", "hunter2-hunter2"))
	log.Error("failed hunter2-hunter2", slog.String("a", "x hunter2-hunter2"),
		slog.Any("err", errors.New("boom hunter2-hunter2")),
		slog.Group("g", slog.String("k", "hunter2-hunter2")))
	if strings.Contains(buf.String(), "hunter2-hunter2") {
		t.Fatalf("secret leaked into log output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), Marker) {
		t.Fatalf("expected marker in output: %s", buf.String())
	}
}
