package slowquery

import (
	"testing"
	"time"
)

func TestParsePostgres(t *testing.T) {
	ts := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	lines := []LogLine{
		{Timestamp: ts, Message: "2026-09-20 10:00:00.100 UTC [1] LOG:  database system is ready to accept connections"},
		{Timestamp: ts.Add(time.Second), Message: "2026-09-20 10:00:01.234 UTC [42] LOG:  duration: 1234.567 ms  statement: SELECT pg_sleep(1.2)"},
		{Timestamp: ts.Add(2 * time.Second), Message: "2026-09-20 10:00:02.001 UTC [43] LOG:  duration: 5.001 ms  execute <unnamed>: SELECT * FROM users WHERE id = $1"},
		{Timestamp: ts.Add(3 * time.Second), Message: "2026-09-20 10:00:03.000 UTC [44] LOG:  checkpoint starting: time"},
	}

	got := ParsePostgres(lines)
	if len(got) != 2 {
		t.Fatalf("ParsePostgres() returned %d entries, want 2: %+v", len(got), got)
	}

	if got[0].DurationMs != 1234.567 {
		t.Errorf("entry 0 DurationMs = %v, want 1234.567", got[0].DurationMs)
	}
	if got[0].Query != "SELECT pg_sleep(1.2)" {
		t.Errorf("entry 0 Query = %q, want %q", got[0].Query, "SELECT pg_sleep(1.2)")
	}
	if !got[0].Timestamp.Equal(ts.Add(time.Second)) {
		t.Errorf("entry 0 Timestamp = %v, want %v", got[0].Timestamp, ts.Add(time.Second))
	}

	if got[1].DurationMs != 5.001 {
		t.Errorf("entry 1 DurationMs = %v, want 5.001", got[1].DurationMs)
	}
	if got[1].Query != "SELECT * FROM users WHERE id = $1" {
		t.Errorf("entry 1 Query = %q, want %q", got[1].Query, "SELECT * FROM users WHERE id = $1")
	}
}

func TestParsePostgres_NoMatches(t *testing.T) {
	lines := []LogLine{
		{Timestamp: time.Now(), Message: "2026-09-20 10:00:00.100 UTC [1] LOG:  database system is ready to accept connections"},
	}
	if got := ParsePostgres(lines); len(got) != 0 {
		t.Errorf("ParsePostgres() = %+v, want no entries", got)
	}
}

func TestParseMySQL(t *testing.T) {
	ts := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	lines := []LogLine{
		{Timestamp: ts, Message: "/usr/sbin/mysqld, Version: 8.0.36 (MySQL Community Server - GPL). started with:"},
		{Timestamp: ts.Add(time.Second), Message: "# Time: 2026-09-20T10:00:01.000000Z"},
		{Timestamp: ts.Add(time.Second), Message: "# User@Host: root[root] @ localhost []  Id:    10"},
		{Timestamp: ts.Add(time.Second), Message: "# Query_time: 2.345678  Lock_time: 0.000123 Rows_sent: 1  Rows_examined: 1000000"},
		{Timestamp: ts.Add(time.Second), Message: "SET timestamp=1704110400;"},
		{Timestamp: ts.Add(time.Second), Message: "SELECT COUNT(*) FROM big_table a JOIN big_table b ON 1=1;"},
		{Timestamp: ts.Add(2 * time.Second), Message: "# Time: 2026-09-20T10:00:02.000000Z"},
		{Timestamp: ts.Add(2 * time.Second), Message: "# User@Host: root[root] @ localhost []  Id:    11"},
		{Timestamp: ts.Add(2 * time.Second), Message: "# Query_time: 0.500000  Lock_time: 0.000050 Rows_sent: 0  Rows_examined: 500"},
		{Timestamp: ts.Add(2 * time.Second), Message: "SET timestamp=1704110401;"},
		{Timestamp: ts.Add(2 * time.Second), Message: "UPDATE accounts"},
		{Timestamp: ts.Add(2 * time.Second), Message: "SET balance = balance + 1;"},
	}

	got := ParseMySQL(lines)
	if len(got) != 2 {
		t.Fatalf("ParseMySQL() returned %d entries, want 2: %+v", len(got), got)
	}

	if got[0].DurationMs != 2345.678 {
		t.Errorf("entry 0 DurationMs = %v, want 2345.678", got[0].DurationMs)
	}
	if got[0].RowsExamined != 1000000 {
		t.Errorf("entry 0 RowsExamined = %v, want 1000000", got[0].RowsExamined)
	}
	wantQuery0 := "SELECT COUNT(*) FROM big_table a JOIN big_table b ON 1=1"
	if got[0].Query != wantQuery0 {
		t.Errorf("entry 0 Query = %q, want %q", got[0].Query, wantQuery0)
	}
	// Timestamp comes from the entry's own "SET timestamp=..." line
	// (1704110400), not the caller-supplied LogLine.Timestamp: it's the
	// engine's own authoritative record of when the query ran.
	wantTS0 := time.Unix(1704110400, 0).UTC()
	if !got[0].Timestamp.Equal(wantTS0) {
		t.Errorf("entry 0 Timestamp = %v, want %v", got[0].Timestamp, wantTS0)
	}

	if got[1].DurationMs != 500 {
		t.Errorf("entry 1 DurationMs = %v, want 500", got[1].DurationMs)
	}
	if got[1].RowsExamined != 500 {
		t.Errorf("entry 1 RowsExamined = %v, want 500", got[1].RowsExamined)
	}
	wantQuery1 := "UPDATE accounts SET balance = balance + 1"
	if got[1].Query != wantQuery1 {
		t.Errorf("entry 1 Query = %q, want %q", got[1].Query, wantQuery1)
	}
	wantTS1 := time.Unix(1704110401, 0).UTC()
	if !got[1].Timestamp.Equal(wantTS1) {
		t.Errorf("entry 1 Timestamp = %v, want %v", got[1].Timestamp, wantTS1)
	}
}

func TestParseMySQL_NoQueryTimeHeader(t *testing.T) {
	lines := []LogLine{
		{Timestamp: time.Now(), Message: "/usr/sbin/mysqld, Version: 8.0.36 (MySQL Community Server - GPL). started with:"},
	}
	if got := ParseMySQL(lines); len(got) != 0 {
		t.Errorf("ParseMySQL() = %+v, want no entries", got)
	}
}

func TestParseMySQL_SkipsUseDBContextLine(t *testing.T) {
	ts := time.Now()
	lines := []LogLine{
		{Timestamp: ts, Message: "# Query_time: 2.300919  Lock_time: 0.000000 Rows_sent: 1  Rows_examined: 1"},
		{Timestamp: ts, Message: "use sq-mysql-test;"},
		{Timestamp: ts, Message: "SET timestamp=1790106791;"},
		{Timestamp: ts, Message: "SELECT SLEEP(2.3);"},
	}
	got := ParseMySQL(lines)
	if len(got) != 1 {
		t.Fatalf("ParseMySQL() returned %d entries, want 1: %+v", len(got), got)
	}
	if got[0].Query != "SELECT SLEEP(2.3)" {
		t.Errorf("Query = %q, want %q (the \"use <db>;\" context line must not leak into it)", got[0].Query, "SELECT SLEEP(2.3)")
	}
}

func TestParseMySQL_TrailingEntryFlushedAtEndOfInput(t *testing.T) {
	ts := time.Now()
	lines := []LogLine{
		{Timestamp: ts, Message: "# Query_time: 1.000000  Lock_time: 0.000000 Rows_sent: 1  Rows_examined: 10"},
		{Timestamp: ts, Message: "SET timestamp=1704110400;"},
		{Timestamp: ts, Message: "SELECT 1;"},
	}
	got := ParseMySQL(lines)
	if len(got) != 1 {
		t.Fatalf("ParseMySQL() returned %d entries, want 1 (last block must flush without a following header): %+v", len(got), got)
	}
	if got[0].Query != "SELECT 1" {
		t.Errorf("entry Query = %q, want %q", got[0].Query, "SELECT 1")
	}
}
