package api

import "testing"

func TestCheckScheduleFires(t *testing.T) {
	if err := checkScheduleFires("0 3 * * *"); err != nil {
		t.Fatalf("daily schedule rejected: %v", err)
	}
	if err := checkScheduleFires("0 0 30 2 *"); err == nil {
		t.Fatal("30 February accepted")
	}
	if err := checkScheduleFires("nope"); err == nil {
		t.Fatal("garbage accepted")
	}
}
