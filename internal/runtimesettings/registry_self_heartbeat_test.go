package runtimesettings

import "testing"

func TestSelfHeartbeatURLCheck(t *testing.T) {
	for _, ok := range []string{"", "https://hc.example/ping/abc"} {
		if err := checkSelfHeartbeatURL(ok); err != nil {
			t.Fatalf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://hc.example/x", "ftp://hc.example", "https://", "not a url", "/relative"} {
		if err := checkSelfHeartbeatURL(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
