package telemetry

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeTable(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"iso timestamp prefix", "2026-10-11T08:15:30.123Z ERROR db connection lost", "db connection lost"},
		{"iso with offset", "2026-10-11 08:15:30+02:00 failed to open file", "failed to open file"},
		{"go log timestamp", "2026/10/11 08:15:30 listen tcp :8080: bind: address already in use", "listen tcp :<num>: bind: address already in use"},
		{"syslog timestamp", "Oct 11 08:15:30 host app[123]: segfault", "host app[<num>]: segfault"},
		{"uuid", "order 3f2b8c1e-4a5d-4e6f-9a7b-0123456789ab not found", "order <uuid> not found"},
		{"ipv4 and port", "dial tcp 10.0.3.17:5432: connection refused", "dial tcp <ip>: connection refused"},
		{"ipv6", "connect to 2001:db8:0:0:0:ff00:42:8329 failed", "connect to <ip> failed"},
		{"hex address", "panic at 0xc000123456 in goroutine 42", "panic at <hex> in goroutine <num>"},
		{"long hex hash", "image sha256 deadbeefcafe1234 missing", "image sha256 <hex> missing"},
		{"durations", "request took 1532ms, upstream 2.5s", "request took <num>, upstream <num>"},
		{"percent", "disk 93% full", "disk <num>% full"},
		{"email", "bounce for alice+x@example.com", "bounce for <email>"},
		{"url path id", "GET /api/v1/users/48213/orders 500", "GET /api/v1/users/<num>/orders <num>"},
		{"bracket level", "[error] upstream timed out (110)", "upstream timed out (<num>)"},
		{"logfmt level", `ts=1 level=error msg="boom 7"`, `ts=<num> msg="boom <num>"`},
		{"prefix colon level", "Error: connect ECONNREFUSED 127.0.0.1:6379", "connect ECONNREFUSED <ip>"},
		{"token", "invalid key abcDEF123456abcDEF123456xyz", "invalid key <token>"},
		{"plain word kept", "internationalization failed", "internationalization failed"},
		{"whitespace", "a   b\t\tc", "a b c"},
		{"stack frame", "at com.acme.Handler.run(Handler.java:128)", "at com.acme.Handler.run(Handler.java:<num>)"},
		{"python traceback", `File "/app/main.py", line 42, in handler`, `File "/app/main.py", line <num>, in handler`},
		{"sql", "duplicate key value violates unique constraint \"users_email_key\" DETAIL: Key (id)=(991)", "duplicate key value violates unique constraint \"users_email_key\" DETAIL: Key (id)=(<num>)"},
		{"negative number", "offset -1 out of range", "offset <num> out of range"},
		{"clock", "tick at 12:04:59.331 slow", "tick at <time> slow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.in); got != tt.want {
				t.Errorf("Normalize(%q)\n got  %q\n want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSignatureOfStructuredUsesMessageField(t *testing.T) {
	a := LogEntry{Structured: true, FieldsJSON: `{"level":"error","msg":"charge failed for user 17","ts":"2026-10-11T00:00:00Z"}`, Message: `{"level":"error"}`}
	b := LogEntry{Structured: true, FieldsJSON: `{"level":"error","msg":"charge failed for user 9042","ts":"2026-10-11T00:05:00Z"}`}
	idA, tplA := SignatureOf(a)
	idB, _ := SignatureOf(b)
	if idA != idB {
		t.Fatalf("same shape must share an id: %s vs %s", idA, idB)
	}
	if tplA != "charge failed for user <num>" {
		t.Fatalf("template = %q", tplA)
	}
}

func TestSignatureTemplateIsBounded(t *testing.T) {
	_, tpl := SignatureOf(LogEntry{Message: strings.Repeat("word ", 500)})
	if len(tpl) > maxTemplateLen {
		t.Fatalf("template length %d exceeds %d", len(tpl), maxTemplateLen)
	}
}

func TestRedactLine(t *testing.T) {
	tests := []struct{ in, want string }{
		{"login password=hunter2 ok", "login password=*** ok"},
		{`{"token": "abc123"}`, `{"token": ***}`},
		{"Authorization: Bearer abc.def.ghi", "Authorization: ***"},
		{"dial postgres://app:s3cret@db:5432/x", "dial postgres://app:***@db:5432/x"},
		{"key sk-live_abcdefghijklmnop1234", "key ***"},
		{"nothing sensitive", "nothing sensitive"},
	}
	for _, tt := range tests {
		if got := RedactLine(tt.in); got != tt.want {
			t.Errorf("RedactLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func seededErrorLogs(base time.Time) []LogEntry {
	var out []LogEntry
	for i := 0; i < 5; i++ {
		out = append(out, LogEntry{
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Message:   "2026-10-11T08:00:00Z ERROR db connection lost to 10.0.0." + string(rune('1'+i)) + ":5432",
		})
	}
	for i := 0; i < 2; i++ {
		out = append(out, LogEntry{
			Timestamp: base.Add(time.Duration(10+i) * time.Minute),
			Message:   "ERROR payment declined for order 99" + string(rune('0'+i)) + " token=abc",
		})
	}
	out = append(out, LogEntry{Timestamp: base, Message: "INFO started"})
	return out
}

func TestAggregateSignaturesTwoShapes(t *testing.T) {
	base := time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC)
	to := base.Add(time.Hour)
	sigs := AggregateSignatures(seededErrorLogs(base), base, to, SignatureOptions{MinLevel: LevelError, Buckets: 6})
	if len(sigs) != 2 {
		t.Fatalf("want 2 signatures, got %d: %+v", len(sigs), sigs)
	}
	if sigs[0].Count != 5 || sigs[1].Count != 2 {
		t.Fatalf("counts = %d, %d; want 5, 2", sigs[0].Count, sigs[1].Count)
	}
	if !sigs[0].FirstSeen.Equal(base) || !sigs[0].LastSeen.Equal(base.Add(4*time.Minute)) {
		t.Fatalf("first/last seen wrong: %v %v", sigs[0].FirstSeen, sigs[0].LastSeen)
	}
	if got := sigs[0].Buckets[0]; got != 5 {
		t.Fatalf("bucket 0 = %d, want 5 (all in the first 10 minutes)", got)
	}
	if strings.Contains(sigs[1].Sample, "abc") {
		t.Fatalf("sample leaks secret: %q", sigs[1].Sample)
	}
}

func TestMarkNewSignatures(t *testing.T) {
	cur := []Signature{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	n := MarkNewSignatures(cur, []Signature{{ID: "b"}})
	if n != 2 || cur[1].New || !cur[0].New || !cur[2].New {
		t.Fatalf("unexpected marking: %d %+v", n, cur)
	}
}
