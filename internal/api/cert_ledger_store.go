package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/store"
)

type certLedgerDB interface {
	SaveCertLedger(ctx context.Context, e store.CertLedgerEntry) error
	LoadCertLedger(ctx context.Context, now time.Time, keep time.Duration) ([]store.CertLedgerEntry, error)
}

type certLedgerBacking struct {
	db  certLedgerDB
	log interface{ Warn(msg string, args ...any) }
}

func (b certLedgerBacking) Save(kind, domain string, at time.Time) {
	if err := b.db.SaveCertLedger(context.Background(), store.CertLedgerEntry{Domain: domain, Kind: kind, At: at}); err != nil {
		b.log.Warn("cert ledger not saved", "domain", domain, "err", err)
	}
}

// attachCertLedger reloads recent attempts so a restart does not reset the issuance budget.
func (rt *Router) attachCertLedger(s any) {
	db, ok := s.(certLedgerDB)
	if !ok {
		return
	}
	now := time.Now()
	rows, err := db.LoadCertLedger(context.Background(), now, certflow.LedgerKeep)
	if err != nil {
		rt.logger.Warn("cert ledger not loaded", "err", err)
		return
	}
	entries := make([]certflow.Entry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, certflow.Entry{Kind: r.Kind, Domain: r.Domain, At: r.At})
	}
	rt.certLedger.Restore(certLedgerBacking{db: db, log: rt.logger}, entries, now)
}
