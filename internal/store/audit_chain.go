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
	// SeqFrom and SeqTo are the chain positions the problem covers (a gap
	// spans several); a range check matches problems by overlap with them.
	SeqFrom int64 `json:"seq_from,omitempty"`
	SeqTo   int64 `json:"seq_to,omitempty"`
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
	// Head and HeadHash are the real top of the chain. ChainOK is the whole
	// chain's verdict; OK covers only the requested range (the same without one).
	HeadHash string `json:"head_hash,omitempty"`
	ChainOK  bool   `json:"chain_ok"`
	// ProblemsOutside counts problems that fall outside a requested range.
	ProblemsOutside int                `json:"problems_outside_range,omitempty"`
	FirstBreak      *AuditChainProblem `json:"first_break,omitempty"`
	// RangeEnd and RangeEndHash are the last position checked in a range run.
	RangeEnd     int64  `json:"range_end,omitempty"`
	RangeEndHash string `json:"range_end_hash,omitempty"`
}

type auditQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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
	rep, _, err := db.verifyChain(ctx, db)
	return rep, err
}

// verifyChain also returns the sequence spans covered by rows and compacted links.
func (db *DB) verifyChain(ctx context.Context, q auditQuerier) (AuditChainReport, [][2]int64, error) {
	key := currentAuditChainKey()
	rep := AuditChainReport{Problems: []AuditChainProblem{}}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE chain_seq IS NULL`).Scan(&rep.Unchained); err != nil {
		return rep, nil, fmt.Errorf("store: count unchained audit rows: %w", err)
	}
	rows, err := q.QueryContext(ctx, `
		SELECT chain_seq, chain_seq, chain_prev, chain_hash, chain_alg, id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind, agent_name, agent_client, action, on_behalf_of, detail, 1
		FROM audit_log WHERE chain_seq IS NOT NULL
		UNION ALL
		SELECT seq_from, seq_to, prev, hash, '', '', '', '', '', '', '', '', 0, '', '', '', '', '', '', '', '', 0 FROM audit_chain_links
		ORDER BY 1`)
	if err != nil {
		return rep, nil, fmt.Errorf("store: read audit chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		last     *chainNode
		nodes    = map[int64]string{}
		ranges   [][2]int64
		firstSeq int64
		spans    [][2]int64
		problem  = func(seq int64, format string, a ...any) {
			rep.Problems = append(rep.Problems, AuditChainProblem{Seq: seq, SeqFrom: seq, SeqTo: seq, Reason: fmt.Sprintf(format, a...)})
		}
		gap = func(from, to int64, format string, a ...any) {
			rep.Problems = append(rep.Problems, AuditChainProblem{Seq: to + 1, SeqFrom: from, SeqTo: to, Reason: fmt.Sprintf(format, a...)})
		}
	)
	for rows.Next() {
		var n chainNode
		var e AuditEntry
		var isRow int
		if err := rows.Scan(&n.from, &n.to, &n.prev, &n.hash, &n.alg, &e.ID, &e.ActorType, &e.ActorID, &e.ActorName, &e.Ability, &e.Method, &e.Path, &e.StatusCode, &e.RemoteAddr, &e.CreatedAt, &e.ClientKind, &e.AgentName, &e.AgentClient, &e.Action, &e.OnBehalfOf, &e.Detail, &isRow); err != nil {
			return rep, nil, fmt.Errorf("store: scan audit chain: %w", err)
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
				gap(last.to+1, n.from-1, "entries %d to %d are missing", last.to+1, n.from-1)
			default:
				problem(n.from, "duplicate chain position %d", n.from)
			}
		}
		if last == nil {
			firstSeq = n.from
		}
		nodes[n.to] = n.hash
		spans = append(spans, [2]int64{n.from, n.to})
		if n.entry == nil {
			ranges = append(ranges, [2]int64{n.from, n.to})
		}
		rep.Head = n.to
		last = &n
	}
	if err := rows.Err(); err != nil {
		return rep, nil, fmt.Errorf("store: iterate audit chain: %w", err)
	}

	cps, err := q.QueryContext(ctx, `SELECT seq, hash, created_at, sig FROM audit_chain_checkpoints ORDER BY seq`)
	if err != nil {
		return rep, nil, fmt.Errorf("store: read audit checkpoints: %w", err)
	}
	defer func() { _ = cps.Close() }()
	for cps.Next() {
		var seq int64
		var hash, at, sig string
		if err := cps.Scan(&seq, &hash, &at, &sig); err != nil {
			return rep, nil, fmt.Errorf("store: scan audit checkpoint: %w", err)
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
			gap(rep.Head+1, seq, "the audit log was cut off: checkpoint %d is newer than its last entry %d", seq, rep.Head)
		case !ok && seq >= firstSeq && last != nil:
			problem(seq, "checkpoint %d points at an entry that is gone", seq)
		}
	}
	if err := cps.Err(); err != nil {
		return rep, nil, fmt.Errorf("store: iterate audit checkpoints: %w", err)
	}
	sort.Slice(rep.Problems, func(i, j int) bool { return rep.Problems[i].Seq < rep.Problems[j].Seq })
	rep.OK = len(rep.Problems) == 0
	rep.ChainOK = rep.OK
	return rep, spans, nil
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

// VerifyAuditChainRange verifies the whole chain (a hash link needs its
// predecessor) in one read transaction, then judges the [from, to] range:
// problems that overlap it, rows inside it, and whether it lies on the chain.
// A zero bound is open; ChainOK still reports the whole chain.
func (db *DB) VerifyAuditChainRange(ctx context.Context, from, to int64) (AuditChainReport, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return AuditChainReport{}, fmt.Errorf("store: begin audit verify: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rep, spans, err := db.verifyChain(ctx, tx)
	if err != nil {
		return rep, err
	}
	if from > 0 || to > 0 {
		lo := max(from, 1)
		hi := to
		if hi == 0 || hi > rep.Head {
			hi = rep.Head
		}
		upper := to
		if upper == 0 {
			upper = 1<<62 - 1
		}
		kept := []AuditChainProblem{}
		for _, p := range rep.Problems {
			if p.SeqFrom <= upper && p.SeqTo >= lo {
				kept = append(kept, p)
			}
		}
		rep.ProblemsOutside = len(rep.Problems) - len(kept)
		if lo > rep.Head || to > rep.Head {
			kept = append(kept, AuditChainProblem{Seq: rep.Head + 1, SeqFrom: rep.Head + 1, SeqTo: max(to, lo), Reason: fmt.Sprintf("range extends past the chain head %d", rep.Head)})
		}
		covered := int64(0)
		for _, sp := range spans {
			if a, b := max(sp[0], lo), min(sp[1], hi); a <= b {
				covered += b - a + 1
			}
		}
		if covered == 0 && lo <= hi {
			kept = append(kept, AuditChainProblem{Seq: lo, SeqFrom: lo, SeqTo: hi, Reason: fmt.Sprintf("no entries exist in range %d to %d", lo, hi)})
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE chain_seq >= ? AND chain_seq <= ?`, lo, hi).Scan(&rep.Checked); err != nil {
			return rep, fmt.Errorf("store: count audit rows in range: %w", err)
		}
		rep.RangeEnd = hi
		hash, found, err := auditHashAt(ctx, tx, hi)
		if err != nil {
			return rep, err
		}
		if found {
			rep.RangeEndHash = hash
		} else if lo <= hi && !inSpans(spans, hi) {
			kept = append(kept, AuditChainProblem{Seq: hi, SeqFrom: hi, SeqTo: hi, Reason: fmt.Sprintf("entry %d at the end of the range is gone", hi)})
		}
		rep.Problems = kept
		rep.OK = len(kept) == 0
	}
	if len(rep.Problems) > 0 {
		first := rep.Problems[0]
		for _, p := range rep.Problems {
			if p.Seq < first.Seq {
				first = p
			}
		}
		rep.FirstBreak = &first
	}
	headHash, _, err := auditHashAt(ctx, tx, rep.Head)
	if err != nil {
		return rep, err
	}
	rep.HeadHash = headHash
	return rep, nil
}

func inSpans(spans [][2]int64, seq int64) bool {
	for _, sp := range spans {
		if seq >= sp[0] && seq <= sp[1] {
			return true
		}
	}
	return false
}

// auditHashAt returns the chain hash at seq from a live row or a compacted link.
func auditHashAt(ctx context.Context, q auditQuerier, seq int64) (string, bool, error) {
	for _, query := range []string{
		`SELECT chain_hash FROM audit_log WHERE chain_seq = ?`,
		`SELECT hash FROM audit_chain_links WHERE seq_to = ?`,
	} {
		var hash string
		err := q.QueryRowContext(ctx, query, seq).Scan(&hash)
		if err == nil {
			return hash, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, fmt.Errorf("store: read audit hash at %d: %w", seq, err)
		}
	}
	return "", false, nil
}
