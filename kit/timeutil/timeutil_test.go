package timeutil

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	want := time.Date(2026, 10, 18, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"2026-10-18T03:04:05Z", want, false},
		{"2026-10-18T03:04:05.123456789Z", want.Add(123456789 * time.Nanosecond), false},
		{"2026-10-18T05:04:05+02:00", want, false},
		{"", time.Time{}, true},
		{"yesterday", time.Time{}, true},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if (err != nil) != c.wantErr {
			t.Fatalf("Parse(%q) err = %v", c.in, err)
		}
		if err == nil && !got.Equal(c.want) {
			t.Errorf("Parse(%q) = %v, want %v", c.in, got, c.want)
		}
		if z := ParseOrZero(c.in); c.wantErr && !z.IsZero() {
			t.Errorf("ParseOrZero(%q) = %v, want zero", c.in, z)
		}
	}
}

func TestParseOptional(t *testing.T) {
	if got, err := ParseOptional(""); got != nil || err != nil {
		t.Fatalf("empty = %v, %v", got, err)
	}
	if _, err := ParseOptional("bad"); err == nil {
		t.Fatal("malformed accepted")
	}
	got, err := ParseOptional("2026-10-18T03:04:05Z")
	if err != nil || got == nil || got.Year() != 2026 {
		t.Fatalf("valid = %v, %v", got, err)
	}
}
