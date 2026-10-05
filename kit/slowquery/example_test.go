package slowquery_test

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/kit/slowquery"
)

func ExampleParsePostgres() {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lines := []slowquery.LogLine{
		{Timestamp: now, Message: "2026-01-01 00:00:00 UTC [41] LOG:  checkpoint starting: time"},
		{Timestamp: now, Message: "2026-01-01 00:00:01 UTC [42] LOG:  duration: 1532.411 ms  statement: SELECT * FROM orders WHERE status = 'open'"},
	}
	for _, e := range slowquery.ParsePostgres(lines) {
		fmt.Printf("%.0f ms: %s\n", e.DurationMs, e.Query)
	}
	// Output:
	// 1532 ms: SELECT * FROM orders WHERE status = 'open'
}

func ExampleParseMySQL() {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lines := []slowquery.LogLine{
		{Timestamp: now, Message: "# Query_time: 2.500000  Lock_time: 0.000100 Rows_sent: 1  Rows_examined: 90000"},
		{Timestamp: now, Message: "SELECT COUNT(*) FROM events;"},
	}
	for _, e := range slowquery.ParseMySQL(lines) {
		fmt.Printf("%.0f ms, %d rows examined: %s\n", e.DurationMs, e.RowsExamined, e.Query)
	}
	// Output:
	// 2500 ms, 90000 rows examined: SELECT COUNT(*) FROM events
}
