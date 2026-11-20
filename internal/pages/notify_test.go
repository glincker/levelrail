package pages

import "testing"

func TestNotifyConfigAllows(t *testing.T) {
	c := NotifyConfig{Enabled: true, OnFailure: true}
	if c.Allows(true) || !c.Allows(false) {
		t.Fatalf("failure-only filter wrong: %+v", c)
	}
	c.Enabled = false
	if c.Allows(false) {
		t.Fatal("disabled config must allow nothing")
	}
	if _, _, err := ParseEvents([]string{"bogus"}); err == nil {
		t.Fatal("unknown event must error")
	}
	s, f, err := ParseEvents([]string{"success", "FAILURE"})
	if err != nil || !s || !f {
		t.Fatalf("parse: %v %v %v", s, f, err)
	}
}
