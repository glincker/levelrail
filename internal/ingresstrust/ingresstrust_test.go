package ingresstrust

import "testing"

func TestValid(t *testing.T) {
	if !Valid(Secret()) {
		t.Fatal("own secret must validate")
	}
	for _, v := range []string{"", "x", Secret() + "x", Secret()[1:]} {
		if Valid(v) {
			t.Errorf("Valid(%q) = true", v)
		}
	}
	if len(Secret()) < 64 {
		t.Error("secret too short")
	}
}
