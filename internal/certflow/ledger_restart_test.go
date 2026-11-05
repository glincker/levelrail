package certflow

import (
	"testing"
	"time"
)

type memBacking struct{ rows []Entry }

func (m *memBacking) Save(kind, domain string, at time.Time) {
	m.rows = append(m.rows, Entry{Kind: kind, Domain: domain, At: at})
}

func TestLedgerRestart(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	var first Ledger
	b := &memBacking{}
	first.Restore(b, nil, now)
	first.RecordIssued("a.example.dev", now.Add(-time.Hour), now)
	first.RecordFailure("a.example.dev", now.Add(-time.Minute), now)
	first.MarkStarted("a.example.dev", now.Add(-2*time.Minute))
	if len(b.rows) != 3 {
		t.Fatalf("saved %d rows, want 3", len(b.rows))
	}

	var second Ledger
	second.Restore(&memBacking{}, b.rows, now)
	if n := second.IssuedSince(now, BudgetWindow); n != 1 {
		t.Fatalf("issued after restart = %d, want 1", n)
	}
	if got := second.Snapshot("a.example.dev").Failed; len(got) != 1 {
		t.Fatalf("failed after restart = %v", got)
	}
	if second.StartedAt("a.example.dev").IsZero() {
		t.Fatal("start time lost")
	}
}
