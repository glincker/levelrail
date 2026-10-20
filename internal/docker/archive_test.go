package docker

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCapReader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		size    int
		max     int64
		wantN   int
		wantErr error
	}{
		{"under the cap", 100, 200, 100, nil},
		{"exactly the cap", 100, 100, 100, nil},
		{"one byte over", 101, 100, 100, ErrArchiveTooLarge},
		{"far over", 1 << 20, 4096, 4096, ErrArchiveTooLarge},
		{"no cap", 1 << 16, 0, 1 << 16, nil},
	}
	for _, tc := range tests {
		got, err := io.ReadAll(CapReader(bytes.NewReader(make([]byte, tc.size)), tc.max))
		if len(got) != tc.wantN || !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: read %d bytes, err %v; want %d, %v", tc.name, len(got), err, tc.wantN, tc.wantErr)
		}
	}
}

func TestCapReadCloserClosesTheSource(t *testing.T) {
	t.Parallel()
	c := &closeProbe{Reader: strings.NewReader("abc")}
	rc := CapReadCloser(c, 10)
	_ = rc.Close()
	if !c.closed {
		t.Error("closing the capped stream did not close the source")
	}
}

type closeProbe struct {
	io.Reader
	closed bool
}

func (c *closeProbe) Close() error { c.closed = true; return nil }
