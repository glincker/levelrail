package spendcap

import "testing"

func TestParseBytes(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"500GiB", 500 << 30, false},
		{"2TB", 2_000_000_000_000, false},
		{"1.5gib", 3 << 29, false},
		{"1024", 1024, false},
		{"10 MB", 10_000_000, false},
		{"", 0, true},
		{"GiB", 0, true},
		{"12 parsecs", 0, true},
		{"-5GB", 0, true},
		{"99999999999TiB", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseBytes(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseBytes(%q) = %d, %v", tc.in, got, err)
		}
	}
}
