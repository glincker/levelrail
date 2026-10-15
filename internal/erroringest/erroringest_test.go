package erroringest

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func exc(typ, val string, frames ...frame) *exceptionValue {
	e := &exceptionValue{Type: typ, Value: val}
	if len(frames) > 0 {
		e.Stacktrace = &struct {
			Frames []frame `json:"frames"`
		}{Frames: frames}
	}
	return e
}

func TestFingerprintGrouping(t *testing.T) {
	yes := true
	a := frame{Module: "app.db", Function: "query", InApp: &yes}
	lib := frame{Module: "lib.net", Function: "dial"}
	other := frame{Module: "app.db", Function: "other", InApp: &yes}
	tests := []struct {
		name     string
		x, y     string
		wantSame bool
	}{
		{"same type and top in-app frame ignore message",
			fingerprint(nil, exc("ValueError", "bad 1", a, lib), ""),
			fingerprint(nil, exc("ValueError", "bad 2", a, lib), ""), true},
		{"different function splits",
			fingerprint(nil, exc("ValueError", "x", a), ""),
			fingerprint(nil, exc("ValueError", "x", other), ""), false},
		{"different type splits",
			fingerprint(nil, exc("ValueError", "x", a), ""),
			fingerprint(nil, exc("KeyError", "x", a), ""), false},
		{"messages differing by number and hex id group",
			fingerprint(nil, nil, "user 42 failed req deadbeefcafe1234"),
			fingerprint(nil, nil, "user 7 failed req 0123456789abcdef"), true},
		{"default-only client fingerprint is ignored",
			fingerprint([]string{"{{ default }}"}, nil, "boom"),
			fingerprint(nil, nil, "boom"), true},
		{"client fingerprint wins",
			fingerprint([]string{"checkout"}, nil, "a"),
			fingerprint([]string{"checkout"}, nil, "b"), true},
		{"client fingerprint differs",
			fingerprint([]string{"checkout"}, nil, "a"),
			fingerprint([]string{"login"}, nil, "a"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.x == tc.y; got != tc.wantSame {
				t.Fatalf("same = %v, want %v", got, tc.wantSame)
			}
		})
	}
}

func envelope(items ...string) []byte {
	return []byte(`{"event_id":"abc","dsn":"x"}` + "\n" + strings.Join(items, "\n") + "\n")
}

func eventItem(payload string) string {
	return fmt.Sprintf(`{"type":"event","length":%d}`, len(payload)) + "\n" + payload
}

func TestParseEnvelope(t *testing.T) {
	ev1 := `{"event_id":"11111111-1111-1111-1111-111111111111","level":"error","timestamp":1700000000.5,` +
		`"release":"v1","environment":"prod","exception":{"values":[{"type":"TypeError","value":"x is undefined",` +
		`"stacktrace":{"frames":[{"filename":"a.js","function":"run","in_app":true}]}}]},` +
		`"request":{"headers":{"Authorization":"Bearer s3cret","Accept":"*/*"},"cookies":"a=b"}}`
	ev2 := `{"message":"hello 99","level":"warning"}`
	tests := []struct {
		name    string
		body    []byte
		want    int
		wantErr bool
	}{
		{"two events", envelope(eventItem(ev1), eventItem(ev2)), 2, false},
		{"unknown and session items are skipped",
			envelope(`{"type":"session"}`+"\n"+`{"sid":"1"}`, `{"type":"attachment","length":3}`+"\n"+"abc",
				`{"type":"transaction"}`+"\n"+`{"type":"transaction"}`, eventItem(ev2)), 1, false},
		{"item without length reads to newline", envelope(`{"type":"event"}` + "\n" + ev2), 1, false},
		{"empty body", nil, 0, true},
		{"not json header", []byte("hello\nworld"), 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEnvelope(tc.body)
			if (err != nil) != tc.wantErr || len(got) != tc.want {
				t.Fatalf("events = %d err = %v, want %d wantErr=%v", len(got), err, tc.want, tc.wantErr)
			}
		})
	}

	got, _ := ParseEnvelope(envelope(eventItem(ev1)))
	e := got[0]
	if e.EventID != "11111111111111111111111111111111" || e.Release != "v1" || e.Environment != "prod" ||
		e.Title != "TypeError: x is undefined" || e.TimestampNs != 1700000000500000000 {
		t.Fatalf("event = %+v", e)
	}
	if strings.Contains(e.Raw, "s3cret") || strings.Contains(e.Raw, "a=b") || !strings.Contains(e.Raw, "Accept") {
		t.Fatalf("raw not scrubbed correctly: %s", e.Raw)
	}
}

func TestParseStoreAndRawBound(t *testing.T) {
	ev, err := ParseStore([]byte(`{"message":{"formatted":"disk full"},"level":"fatal"}`))
	if err != nil || ev.Title != "disk full" || ev.Level != "fatal" {
		t.Fatalf("ev = %+v err = %v", ev, err)
	}
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	_, _ = zw.Write([]byte(`{"message":"legacy"}`))
	_ = zw.Close()
	ev, err = ParseStore([]byte(base64.StdEncoding.EncodeToString(zb.Bytes())))
	if err != nil || ev.Title != "legacy" {
		t.Fatalf("legacy ev = %+v err = %v", ev, err)
	}
	if _, err := ParseStore([]byte(`{"level":"error"}`)); err == nil {
		t.Fatal("an event with neither message nor exception must be rejected")
	}

	big := fmt.Sprintf(`{"message":"big","breadcrumbs":[%q],"extra":{"k":%q}}`, strings.Repeat("a", 40<<10), strings.Repeat("b", 40<<10))
	ev, err = ParseStore([]byte(big))
	if err != nil || len(ev.Raw) > MaxRawBytes || !strings.Contains(ev.Raw, `"big"`) {
		t.Fatalf("raw len = %d err = %v", len(ev.Raw), err)
	}
}
