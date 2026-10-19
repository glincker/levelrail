package httpx

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReadAll(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		limit   int64
		wantErr bool
	}{
		{"under", "abc", 10, false},
		{"exact", "abcde", 5, false},
		{"over", "abcdef", 5, true},
		{"empty", "", 5, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ReadAll(strings.NewReader(c.body), c.limit)
			if c.wantErr {
				if !errors.Is(err, ErrBodyTooLarge) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || string(got) != c.body {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestReadBody(t *testing.T) {
	got, err := ReadBody(strings.NewReader("ok"))
	if err != nil || string(got) != "ok" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient(5 * time.Second)
	if c.Timeout != 5*time.Second || c.Transport == nil {
		t.Fatalf("unexpected client %+v", c)
	}
}
