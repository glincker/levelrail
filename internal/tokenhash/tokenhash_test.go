package tokenhash

import "testing"

func TestSumAndMatches(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, c := range cases {
		if got := Sum(c.in); got != c.want {
			t.Errorf("Sum(%q) = %s, want %s", c.in, got, c.want)
		}
		if !Matches(c.in, c.want) {
			t.Errorf("Matches(%q) = false", c.in)
		}
	}
	if Matches("abc", "deadbeef") {
		t.Error("Matches accepted a wrong hash")
	}
}
