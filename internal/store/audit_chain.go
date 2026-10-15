package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
)

// AuditEntryHash is the chain hash of e: SHA-256 over every recorded field,
// each length-prefixed so field boundaries cannot be shifted, plus Seq and
// PrevHash. EntryHash itself is excluded.
func AuditEntryHash(e AuditEntry) string {
	h := sha256.New()
	fields := []string{
		"audit-chain-v1", strconv.FormatInt(e.Seq, 10), e.PrevHash, e.ID, e.ActorType, e.ActorID, e.ActorName,
		e.Ability, e.Method, e.Path, strconv.Itoa(e.StatusCode), e.RemoteAddr, e.CreatedAt, e.ClientKind,
		e.AgentName, e.AgentClient, e.Action, e.OnBehalfOf,
	}
	for _, f := range fields {
		h.Write([]byte(strconv.Itoa(len(f))))
		h.Write([]byte{':'})
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AuditChainBreak locates the first row that fails verification.
type AuditChainBreak struct {
	Seq    int64
	ID     string
	Reason string
}

// AuditChainReport is the result of VerifyAuditChain.
type AuditChainReport struct {
	Chained   int64
	Unchained int64
	HeadSeq   int64
	HeadHash  string
	// FirstBroken is nil when every chained row verified.
	FirstBroken *AuditChainBreak
}

// Audit chain break reasons.
const (
	AuditBreakHashMismatch = "entry_hash_mismatch"
	AuditBreakPrevMismatch = "prev_hash_mismatch"
	AuditBreakSeqGap       = "seq_gap"
	AuditBreakTruncated    = "tail_truncated"
)

const auditColumnsChained = `id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind, agent_name, agent_client, action, on_behalf_of, seq, prev_hash, entry_hash`

type auditRowScanner interface {
	Scan(dest ...any) error
}

func scanChainedAuditEntry(s auditRowScanner) (AuditEntry, error) {
	var e AuditEntry
	err := s.Scan(&e.ID, &e.ActorType, &e.ActorID, &e.ActorName, &e.Ability, &e.Method, &e.Path, &e.StatusCode, &e.RemoteAddr, &e.CreatedAt, &e.ClientKind, &e.AgentName, &e.AgentClient, &e.Action, &e.OnBehalfOf, &e.Seq, &e.PrevHash, &e.EntryHash)
	return e, err
}

// VerifyAuditChain walks every chained row in seq order. The oldest
// retained row anchors the walk, since retention may have removed what
// came before it; every later row must continue the chain, and the last
// one must match the stored head.
func (db *DB) VerifyAuditChain(ctx context.Context) (AuditChainReport, error) {
	var rep AuditChainReport
	if err := db.QueryRowContext(ctx, `SELECT seq, hash FROM audit_chain_head WHERE id = 1`).Scan(&rep.HeadSeq, &rep.HeadHash); err != nil {
		return rep, fmt.Errorf("store: read audit chain head: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE seq = 0`).Scan(&rep.Unchained); err != nil {
		return rep, fmt.Errorf("store: count unchained audit entries: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT `+auditColumnsChained+` FROM audit_log WHERE seq > 0 ORDER BY seq ASC`)
	if err != nil {
		return rep, fmt.Errorf("store: walk audit chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var prev *AuditEntry
	for rows.Next() {
		e, err := scanChainedAuditEntry(rows)
		if err != nil {
			return rep, fmt.Errorf("store: scan audit chain row: %w", err)
		}
		rep.Chained++
		if rep.FirstBroken == nil {
			rep.FirstBroken = checkAuditLink(prev, e)
		}
		cp := e
		prev = &cp
	}
	if err := rows.Err(); err != nil {
		return rep, fmt.Errorf("store: iterate audit chain: %w", err)
	}
	if rep.FirstBroken == nil {
		switch {
		case prev != nil && (prev.Seq != rep.HeadSeq || prev.EntryHash != rep.HeadHash):
			rep.FirstBroken = &AuditChainBreak{Seq: prev.Seq, ID: prev.ID, Reason: AuditBreakTruncated}
		case prev == nil && rep.HeadSeq > 0:
			rep.FirstBroken = &AuditChainBreak{Seq: rep.HeadSeq, Reason: AuditBreakTruncated}
		}
	}
	return rep, nil
}

func checkAuditLink(prev *AuditEntry, e AuditEntry) *AuditChainBreak {
	if prev != nil {
		if e.Seq != prev.Seq+1 {
			return &AuditChainBreak{Seq: e.Seq, ID: e.ID, Reason: AuditBreakSeqGap}
		}
		if e.PrevHash != prev.EntryHash {
			return &AuditChainBreak{Seq: e.Seq, ID: e.ID, Reason: AuditBreakPrevMismatch}
		}
	}
	if AuditEntryHash(e) != e.EntryHash {
		return &AuditChainBreak{Seq: e.Seq, ID: e.ID, Reason: AuditBreakHashMismatch}
	}
	return nil
}

// ListAuditEntriesAfterSeq returns up to limit chained rows with seq above
// afterSeq, oldest first: the cursor a streaming sink advances.
func (db *DB) ListAuditEntriesAfterSeq(ctx context.Context, afterSeq int64, limit int) ([]AuditEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+auditColumnsChained+` FROM audit_log WHERE seq > ? ORDER BY seq ASC LIMIT ?`, afterSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list audit entries after seq: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AuditEntry
	for rows.Next() {
		e, err := scanChainedAuditEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan audit entry row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate audit entries after seq: %w", err)
	}
	return out, nil
}

// AuditChainHeadSeq returns the newest assigned seq, 0 before any chained row.
func (db *DB) AuditChainHeadSeq(ctx context.Context) (int64, error) {
	var seq int64
	if err := db.QueryRowContext(ctx, `SELECT seq FROM audit_chain_head WHERE id = 1`).Scan(&seq); err != nil {
		return 0, fmt.Errorf("store: read audit chain head seq: %w", err)
	}
	return seq, nil
}
