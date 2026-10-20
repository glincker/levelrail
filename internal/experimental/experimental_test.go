package experimental

import (
	"strings"
	"testing"
)

func TestDefaultOffAndOnWhenListed(t *testing.T) {
	for _, f := range All() {
		t.Run(string(f), func(t *testing.T) {
			t.Setenv(EnvVar, "")
			Reset()
			if Enabled(f) {
				t.Fatalf("%s enabled by default", f)
			}
			t.Setenv(EnvVar, " "+strings.ToUpper(string(f))+" , ")
			Reset()
			if !Enabled(f) {
				t.Fatalf("%s not enabled when listed", f)
			}
			for _, other := range All() {
				if other != f && Enabled(other) {
					t.Fatalf("%s leaked on with only %s listed", other, f)
				}
			}
		})
	}
	Reset()
}

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", 0, false},
		{"iac,ai-chat", 2, false},
		{"iac,nope", 0, true},
	}
	for _, tc := range tests {
		got, err := Parse(tc.in)
		if (err != nil) != tc.wantErr || len(got) != tc.want {
			t.Errorf("Parse(%q) = %v, %v", tc.in, got, err)
		}
	}
}

func TestLenientLoadIgnoresTypos(t *testing.T) {
	t.Setenv(EnvVar, "iac,typo")
	Reset()
	defer Reset()
	if !Enabled(IaC) || Enabled(AIChat) {
		t.Fatal("lenient load wrong")
	}
	if Validate() == nil {
		t.Fatal("Validate should flag the typo")
	}
}
