package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuditAnchorVersion is the export record format. It is bumped only when a field changes meaning.
const AuditAnchorVersion = 1

// AuditAnchor is one off-box record of the audit chain head. It carries a
// sequence, a hash and a signature, never row content or the signing key.
type AuditAnchor struct {
	Version int    `json:"v"`
	Seq     int64  `json:"seq"`
	Hash    string `json:"hash"`
	At      string `json:"at"`
	Sig     string `json:"sig"`
}

func auditAnchorSigPayload(seq int64, hash, at string) []byte {
	return []byte(fmt.Sprintf("anchor|%d|%s|%s", seq, hash, at))
}

// LatestAuditAnchor signs the current chain head with the checkpoint key.
// ok is false when the log has no chained row yet.
func (db *DB) LatestAuditAnchor(ctx context.Context) (a AuditAnchor, ok bool, err error) {
	var seq int64
	var hash string
	switch scanErr := db.QueryRowContext(ctx, auditChainHeadQuery).Scan(&seq, &hash); {
	case errors.Is(scanErr, sql.ErrNoRows):
		return AuditAnchor{}, false, nil
	case scanErr != nil:
		return AuditAnchor{}, false, fmt.Errorf("store: read audit chain head: %w", scanErr)
	}
	key := currentAuditChainKey()
	alg := auditChainAlg(key)
	at := FormatAuditTime(time.Now())
	sig := alg + ":" + auditDigest(key, alg, auditAnchorSigPayload(seq, hash, at))
	return AuditAnchor{Version: AuditAnchorVersion, Seq: seq, Hash: hash, At: at, Sig: sig}, true, nil
}

// AuditAnchorReport is the result of CheckAuditAnchors.
type AuditAnchorReport struct {
	OK         bool                `json:"ok"`
	Checked    int                 `json:"checked"`
	Matched    int                 `json:"matched"`
	Compacted  int                 `json:"compacted"`
	Unverified int                 `json:"unverified"`
	LiveHead   int64               `json:"live_head"`
	Problems   []AuditChainProblem `json:"problems"`
}

// CheckAuditAnchors confirms the live chain still holds every anchored head.
// A head inside a purged range cannot be compared and is counted as compacted.
func (db *DB) CheckAuditAnchors(ctx context.Context, anchors []AuditAnchor) (AuditAnchorReport, error) {
	rep := AuditAnchorReport{Problems: []AuditChainProblem{}}
	key := currentAuditChainKey()
	var headHash string
	switch err := db.QueryRowContext(ctx, auditChainHeadQuery).Scan(&rep.LiveHead, &headHash); {
	case errors.Is(err, sql.ErrNoRows):
		rep.LiveHead = 0
	case err != nil:
		return rep, fmt.Errorf("store: read audit chain head: %w", err)
	}
	problem := func(seq int64, format string, a ...any) {
		rep.Problems = append(rep.Problems, AuditChainProblem{Seq: seq, Reason: fmt.Sprintf(format, a...)})
	}
	for _, a := range anchors {
		rep.Checked++
		alg, mac, _ := strings.Cut(a.Sig, ":")
		switch {
		case alg == auditAlgKeyed && len(key) == 0:
			rep.Unverified++
		case alg != auditAlgKeyed && alg != auditAlgPlain:
			problem(a.Seq, "anchor %d has an unknown signature algorithm", a.Seq)
			continue
		case auditDigest(key, alg, auditAnchorSigPayload(a.Seq, a.Hash, a.At)) != mac:
			problem(a.Seq, "anchor %d signature is invalid, the export was edited or signed with another key", a.Seq)
			continue
		}
		hash, found, inLink, err := db.auditHashAt(ctx, a.Seq)
		if err != nil {
			return rep, err
		}
		switch {
		case found && hash == a.Hash:
			rep.Matched++
		case found:
			problem(a.Seq, "entry %d was rewritten: its hash differs from the anchored head", a.Seq)
		case inLink:
			rep.Compacted++
		case a.Seq > rep.LiveHead:
			problem(a.Seq, "the audit log was rolled back: anchored head %d is newer than the live head %d", a.Seq, rep.LiveHead)
		default:
			problem(a.Seq, "entry %d is gone from the live chain", a.Seq)
		}
	}
	rep.OK = len(rep.Problems) == 0
	return rep, nil
}

// auditHashAt returns the chain hash at seq. inLink is true when seq falls
// inside a purged range, whose interior hashes are not kept.
func (db *DB) auditHashAt(ctx context.Context, seq int64) (hash string, found, inLink bool, err error) {
	switch e := db.QueryRowContext(ctx, `SELECT chain_hash FROM audit_log WHERE chain_seq = ?`, seq).Scan(&hash); {
	case e == nil:
		return hash, true, false, nil
	case !errors.Is(e, sql.ErrNoRows):
		return "", false, false, fmt.Errorf("store: read audit chain entry: %w", e)
	}
	var from, to int64
	switch e := db.QueryRowContext(ctx, `SELECT seq_from, seq_to, hash FROM audit_chain_links WHERE seq_from <= ? AND seq_to >= ?`, seq, seq).Scan(&from, &to, &hash); {
	case errors.Is(e, sql.ErrNoRows):
		return "", false, false, nil
	case e != nil:
		return "", false, false, fmt.Errorf("store: read audit chain link: %w", e)
	}
	if to == seq {
		return hash, true, false, nil
	}
	return "", false, true, nil
}
