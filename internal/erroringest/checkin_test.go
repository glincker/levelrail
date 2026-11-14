package erroringest

import "testing"

func TestParseEnvelopeItemsCheckIns(t *testing.T) {
	body := "{}\n" +
		`{"type":"check_in"}` + "\n" + `{"monitor_slug":"nightly","status":"in_progress"}` + "\n" +
		`{"type":"check_in"}` + "\n" + `{"monitor_slug":"nightly","status":"bogus"}` + "\n" +
		`{"type":"check_in"}` + "\n" + `{"monitor_slug":"nightly","status":"error"}` + "\n" +
		`{"type":"event"}` + "\n" + `{"event_id":"abc","message":"boom"}` + "\n"
	evs, cis, err := ParseEnvelopeItems([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || len(cis) != 2 || cis[0].Status != "in_progress" || cis[1].Status != "error" {
		t.Fatalf("events=%d checkins=%+v", len(evs), cis)
	}
}
