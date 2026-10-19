package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	auditCheckpointEvery = 64
	auditAlgKeyed        = "k"
	auditAlgPlain        = "h"
)

var auditChainKey atomic.Pointer[[]byte]

// SetAuditChainKey sets the HMAC key new audit rows are chained under. Rows
// written before it is set are chained with a plain hash and still verify.
func SetAuditChainKey(key []byte) {
	if len(key) == 0 {
		auditChainKey.Store(nil)
		return
	}
	k := append([]byte(nil), key...)
	auditChainKey.Store(&k)
}

func currentAuditChainKey() []byte {
	if k := auditChainKey.Load(); k != nil {
		return *k
	}
	return nil
}

type auditChainDB interface {
	auditExecer
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func auditChainPayload(prev string, e AuditEntry) []byte {
	fields, _ := json.Marshal([]string{
		e.ID, e.ActorType, e.ActorID, e.ActorName, e.Ability, e.Method, e.Path, strconv.Itoa(e.StatusCode),
		e.RemoteAddr, e.CreatedAt, e.ClientKind, e.AgentName, e.AgentClient, e.Action, e.OnBehalfOf, e.Detail,
	})
	return append([]byte(prev+"\n"), fields...)
}

func auditDigest(key []byte, alg string, payload []byte) string {
	if alg == auditAlgKeyed {
		mac := hmac.New(sha256.New, key)
		mac.Write(payload)
		return hex.EncodeToString(mac.Sum(nil))
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func auditChainAlg(key []byte) string {
	if len(key) > 0 {
		return auditAlgKeyed
	}
	return auditAlgPlain
}

const auditChainHeadQuery = `
	SELECT seq, hash FROM (
		SELECT chain_seq AS seq, chain_hash AS hash FROM audit_log WHERE chain_seq IS NOT NULL
		UNION ALL
		SELECT seq_to AS seq, hash FROM audit_chain_links
	) ORDER BY seq DESC LIMIT 1`

// chainAuditEntry links e to the previous row and returns its chain columns.
// SQLite runs with one connection, so a write never interleaves with another.
func chainAuditEntry(ctx context.Context, x auditChainDB, e AuditEntry) (seq int64, prev, hash, alg string, err error) {
	var prevSeq int64
	switch scanErr := x.QueryRowContext(ctx, auditChainHeadQuery).Scan(&prevSeq, &prev); {
	case errors.Is(scanErr, sql.ErrNoRows):
		prevSeq, prev = 0, ""
	case scanErr != nil:
		return 0, "", "", "", fmt.Errorf("store: read audit chain head: %w", scanErr)
	}
	key := currentAuditChainKey()
	alg = auditChainAlg(key)
	hash = auditDigest(key, alg, auditChainPayload(prev, e))
	return prevSeq + 1, prev, hash, alg, nil
}

func writeAuditCheckpoint(ctx context.Context, x auditExecer, seq int64, hash string) error {
	key := currentAuditChainKey()
	alg := auditChainAlg(key)
	at := FormatAuditTime(time.Now())
	sig := alg + ":" + auditDigest(key, alg, []byte(fmt.Sprintf("checkpoint|%d|%s|%s", seq, hash, at)))
	_, err := x.ExecContext(ctx, `INSERT OR IGNORE INTO audit_chain_checkpoints (seq, hash, created_at, sig) VALUES (?, ?, ?, ?)`, seq, hash, at, sig)
	if err != nil {
		return fmt.Errorf("store: write audit checkpoint %d: %w", seq, err)
	}
	return nil
}

// AuditChainProblem is one reason the audit log failed verification.
type AuditChainProblem struct {
	Seq    int64  `json:"seq"`
	Reason string `json:"reason"`
}

// AuditChainReport is the result of VerifyAuditChain.
type AuditChainReport struct {
	OK          bool                `json:"ok"`
	Checked     int                 `json:"checked"`
	Head        int64               `json:"head"`
	Checkpoints int                 `json:"checkpoints"`
	Unverified  int                 `json:"unverified"`
	Unchained   int                 `json:"unchained"`
	Problems    []AuditChainProblem `json:"problems"`
}

type chainNode struct {
	from, to   int64
	prev, hash string
	alg        string
	entry      *AuditEntry
}

// VerifyAuditChain walks the audit hash chain and its signed checkpoints and
// reports every row that was edited, removed or cut off the end.
func (db *DB) VerifyAuditChain(ctx context.Context) (AuditChainReport, error) {
	key := currentAuditChainKey()
	rep := AuditChainReport{Problems: []AuditChainProblem{}}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE chain_seq IS NULL`).Scan(&rep.Unchained); err != nil {
		return rep, fmt.Errorf("store: count unchained audit rows: %w", err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT chain_seq, chain_seq, chain_prev, chain_hash, chain_alg, id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind, agent_name, agent_client, action, on_behalf_of, detail, 1
		FROM audit_log WHERE chain_seq IS NOT NULL
		UNION ALL
		SELECT seq_from, seq_to, prev, hash, '', '', '', '', '', '', '', '', 0, '', '', '', '', '', '', '', '', 0 FROM audit_chain_links
		ORDER BY 1`)
	if err != nil {
		return rep, fmt.Errorf("store: read audit chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		last     *chainNode
		nodes    = map[int64]string{}
		ranges   [][2]int64
		firstSeq int64
		problem  = func(seq int64, format string, a ...any) {
			rep.Problems = append(rep.Problems, AuditChainProblem{Seq: seq, Reason: fmt.Sprintf(format, a...)})
		}
	)
	for rows.Next() {
		var n chainNode
		var e AuditEntry
		var isRow int
		if err := rows.Scan(&n.from, &n.to, &n.prev, &n.hash, &n.alg, &e.ID, &e.ActorType, &e.ActorID, &e.ActorName, &e.Ability, &e.Method, &e.Path, &e.StatusCode, &e.RemoteAddr, &e.CreatedAt, &e.ClientKind, &e.AgentName, &e.AgentClient, &e.Action, &e.OnBehalfOf, &e.Detail, &isRow); err != nil {
			return rep, fmt.Errorf("store: scan audit chain: %w", err)
		}
		if isRow == 1 {
			n.entry = &e
			rep.Checked++
			switch {
			case n.alg == auditAlgKeyed && len(key) == 0:
				rep.Unverified++
			case n.alg != auditAlgKeyed && n.alg != auditAlgPlain:
				problem(n.from, "unknown chain algorithm %q", n.alg)
			default:
				if want := auditDigest(key, n.alg, auditChainPayload(n.prev, e)); want != n.hash {
					problem(n.from, "row %s was modified: its hash does not match its content", e.ID)
				}
			}
		}
		if last != nil {
			switch {
			case n.from == last.to+1:
				if n.prev != last.hash {
					problem(n.from, "chain broken: entry %d does not follow entry %d", n.from, last.to)
				}
			case n.from > last.to+1:
				problem(n.from, "entries %d to %d are missing", last.to+1, n.from-1)
			default:
				problem(n.from, "duplicate chain position %d", n.from)
			}
		}
		if last == nil {
			firstSeq = n.from
		}
		nodes[n.to] = n.hash
		if n.entry == nil {
			ranges = append(ranges, [2]int64{n.from, n.to})
		}
		rep.Head = n.to
		last = &n
	}
	if err := rows.Err(); err != nil {
		return rep, fmt.Errorf("store: iterate audit chain: %w", err)
	}

	cps, err := db.QueryContext(ctx, `SELECT seq, hash, created_at, sig FROM audit_chain_checkpoints ORDER BY seq`)
	if err != nil {
		return rep, fmt.Errorf("store: read audit checkpoints: %w", err)
	}
	defer func() { _ = cps.Close() }()
	for cps.Next() {
		var seq int64
		var hash, at, sig string
		if err := cps.Scan(&seq, &hash, &at, &sig); err != nil {
			return rep, fmt.Errorf("store: scan audit checkpoint: %w", err)
		}
		rep.Checkpoints++
		alg, mac, _ := strings.Cut(sig, ":")
		if alg == auditAlgKeyed && len(key) == 0 {
			rep.Unverified++
		} else if auditDigest(key, alg, []byte(fmt.Sprintf("checkpoint|%d|%s|%s", seq, hash, at))) != mac {
			problem(seq, "checkpoint %d signature is invalid", seq)
		}
		inLink := false
		for _, r := range ranges {
			inLink = inLink || (seq >= r[0] && seq <= r[1])
		}
		switch got, ok := nodes[seq]; {
		case inLink && !ok:
		case ok && got != hash:
			problem(seq, "checkpoint %d does not match the chain", seq)
		case !ok && seq > rep.Head:
			problem(seq, "the audit log was cut off: checkpoint %d is newer than its last entry %d", seq, rep.Head)
		case !ok && seq >= firstSeq && last != nil:
			problem(seq, "checkpoint %d points at an entry that is gone", seq)
		}
	}
	if err := cps.Err(); err != nil {
		return rep, fmt.Errorf("store: iterate audit checkpoints: %w", err)
	}
	sort.Slice(rep.Problems, func(i, j int) bool { return rep.Problems[i].Seq < rep.Problems[j].Seq })
	rep.OK = len(rep.Problems) == 0
	return rep, nil
}

// securityAuditPredicate matches the audit rows that outlive the default
// retention: sign-in, step-up, token and policy events.
const securityAuditPredicate = `(
	action LIKE 'step_up.%' OR action LIKE 'device_login%' OR action LIKE 'login_code%' OR action LIKE 'new_device%'
	OR action LIKE 'security%' OR action LIKE 'sso.%' OR action LIKE 'scim.%' OR action LIKE 'api_token.%'
	OR action IN ('env.revealed', 'env.exported', 'sensitive_read')
	OR path LIKE '/api/v1/auth/tokens%' OR path LIKE '/api/v1/security/%' OR path LIKE '/api/v1/auth/login-approvals/%'
)`

// DeleteAuditEntriesTiered removes audit rows older than cutoff, except
// security-family rows, which stay until securityCutoff. The chain position of
// every removed row is folded into a compact link so verification still sees
// an unbroken chain.
func (db *DB) DeleteAuditEntriesTiered(ctx context.Context, cutoff, securityCutoff time.Time) (int64, error) {
	if securityCutoff.After(cutoff) {
		securityCutoff = cutoff
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: begin audit purge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	where := `created_at < ? AND (created_at < ? OR NOT ` + securityAuditPredicate + `)`
	args := []any{FormatAuditTime(cutoff), FormatAuditTime(securityCutoff)}
	rows, err := tx.QueryContext(ctx, `SELECT chain_seq, chain_prev, chain_hash FROM audit_log WHERE chain_seq IS NOT NULL AND `+where+` ORDER BY chain_seq`, args...)
	if err != nil {
		return 0, fmt.Errorf("store: select purged audit chain: %w", err)
	}
	type run struct {
		from, to   int64
		prev, hash string
	}
	var runs []run
	for rows.Next() {
		var seq int64
		var prev, hash string
		if err := rows.Scan(&seq, &prev, &hash); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("store: scan purged audit chain: %w", err)
		}
		if n := len(runs); n > 0 && runs[n-1].to+1 == seq {
			runs[n-1].to, runs[n-1].hash = seq, hash
			continue
		}
		runs = append(runs, run{from: seq, to: seq, prev: prev, hash: hash})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("store: iterate purged audit chain: %w", err)
	}
	_ = rows.Close()

	for _, r := range runs {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO audit_chain_links (seq_from, seq_to, prev, hash) VALUES (?, ?, ?, ?)`, r.from, r.to, r.prev, r.hash); err != nil {
			return 0, fmt.Errorf("store: save audit chain link: %w", err)
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM audit_log WHERE `+where, args...)
	if err != nil {
		return 0, fmt.Errorf("store: delete audit entries older than %s: %w", cutoff, err)
	}
	if err := mergeAuditLinks(ctx, tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: commit audit purge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: count deleted audit entries: %w", err)
	}
	return n, nil
}

// mergeAuditLinks joins link ranges that touch, keeping the table small.
func mergeAuditLinks(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT seq_from, seq_to, prev, hash FROM audit_chain_links ORDER BY seq_from`)
	if err != nil {
		return fmt.Errorf("store: read audit chain links: %w", err)
	}
	type link struct {
		from, to   int64
		prev, hash string
	}
	var in, out []link
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.from, &l.to, &l.prev, &l.hash); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store: scan audit chain link: %w", err)
		}
		in = append(in, l)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("store: iterate audit chain links: %w", err)
	}
	_ = rows.Close()
	for _, l := range in {
		if n := len(out); n > 0 && out[n-1].to+1 == l.from {
			out[n-1].to, out[n-1].hash = l.to, l.hash
			continue
		}
		out = append(out, l)
	}
	if len(out) == len(in) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM audit_chain_links`); err != nil {
		return fmt.Errorf("store: reset audit chain links: %w", err)
	}
	for _, l := range out {
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_chain_links (seq_from, seq_to, prev, hash) VALUES (?, ?, ?, ?)`, l.from, l.to, l.prev, l.hash); err != nil {
			return fmt.Errorf("store: rewrite audit chain link: %w", err)
		}
	}
	return nil
}
