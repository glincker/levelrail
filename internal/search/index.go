package search

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// DB is the slice of *sql.DB the service uses.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

const candidateLimit = 500

func docHash(d Doc) string {
	h := sha256.New()
	for _, s := range []string{string(d.Kind), d.Title, d.Body, strings.Join(d.Tags, "|"), d.App, d.Env, d.Node,
		d.Prefix, d.Name, d.Ability, d.Href, d.ActionID, fmt.Sprint(d.Confirm), fmt.Sprint(d.UpdatedAt.Unix())} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func tagColumn(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "|" + strings.ToLower(strings.Join(tags, "|")) + "|"
}

// replaceKind makes the index hold exactly docs for kind, touching only rows
// whose content changed.
func replaceKind(ctx context.Context, db DB, kind Kind, docs []Doc) error {
	for _, d := range docs {
		if d.Kind != kind {
			return fmt.Errorf("search: provider %q produced a %q doc", kind, d.Kind)
		}
		if err := d.Validate(); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("search: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT ref, hash FROM search_docs WHERE kind = ?`, string(kind))
	if err != nil {
		return fmt.Errorf("search: list existing: %w", err)
	}
	for rows.Next() {
		var ref, hash string
		if err := rows.Scan(&ref, &hash); err != nil {
			_ = rows.Close()
			return fmt.Errorf("search: scan existing: %w", err)
		}
		existing[ref] = hash
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("search: read existing: %w", err)
	}
	_ = rows.Close()

	seen := make(map[string]struct{}, len(docs))
	for _, d := range docs {
		seen[d.Ref] = struct{}{}
		h := docHash(d)
		if existing[d.Ref] == h {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO search_docs
			(ref, kind, title, body, tags, app, env, node, prefix, name, ability, confirm, href, action_id, updated_at, hash)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(ref) DO UPDATE SET kind=excluded.kind, title=excluded.title, body=excluded.body,
			tags=excluded.tags, app=excluded.app, env=excluded.env, node=excluded.node, prefix=excluded.prefix,
			name=excluded.name, ability=excluded.ability, confirm=excluded.confirm, href=excluded.href,
			action_id=excluded.action_id, updated_at=excluded.updated_at, hash=excluded.hash`,
			d.Ref, string(d.Kind), d.Title, d.Body, tagColumn(d.Tags), strings.ToLower(d.App), strings.ToLower(d.Env),
			strings.ToLower(d.Node), d.Prefix, d.Name, d.Ability, boolInt(d.Confirm), d.Href, d.ActionID,
			d.UpdatedAt.Unix(), h)
		if err != nil {
			return fmt.Errorf("search: upsert %q: %w", d.Ref, err)
		}
	}
	for ref := range existing {
		if _, ok := seen[ref]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_docs WHERE ref = ?`, ref); err != nil {
			return fmt.Errorf("search: delete %q: %w", ref, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("search: commit: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type candidate struct {
	doc       Doc
	relevance float64
}

func likeArg(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.ToLower(s)) + "%"
}

func ftsPhrase(term string) string {
	return `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
}

// fetchCandidates returns the best-ranked raw rows for q. It is deliberately
// unexported: the only reader of the index is Service.Search, which filters
// through the Authorizer before ranking.
func fetchCandidates(ctx context.Context, db DB, q Query, kind Kind, mode matchMode) ([]candidate, error) {
	var (
		where []string
		args  []any
	)
	var long, short []string
	for _, t := range q.Text {
		if len([]rune(t)) >= 3 {
			long = append(long, t)
		} else {
			short = append(short, t)
		}
	}
	sel := `SELECT d.ref, d.kind, d.title, d.body, d.tags, d.app, d.env, d.node, d.prefix, d.name, d.ability, d.confirm, d.href, d.action_id, d.updated_at`
	from := ` FROM search_docs d`
	order := ` ORDER BY d.updated_at DESC`
	if len(long) > 0 {
		sel += `, bm25(search_docs_fts, 5.0, 1.0)`
		from = ` FROM search_docs_fts JOIN search_docs d ON d.id = search_docs_fts.rowid`
		order = ` ORDER BY bm25(search_docs_fts, 5.0, 1.0)`
		phrases := make([]string, len(long))
		for i, t := range long {
			phrases[i] = ftsPhrase(t)
		}
		join := " AND "
		if mode == matchLoose {
			join = " OR "
			phrases = trigramPhrases(long)
		}
		where = append(where, `search_docs_fts MATCH ?`)
		args = append(args, strings.Join(phrases, join))
	} else {
		sel += `, 0.0`
	}
	for _, t := range short {
		where = append(where, `(lower(d.title) LIKE ? ESCAPE '\' OR lower(d.body) LIKE ? ESCAPE '\')`)
		args = append(args, likeArg(t), likeArg(t))
	}
	if q.App != "" {
		where = append(where, `(d.app LIKE ? ESCAPE '\' OR (d.kind = 'app' AND lower(d.name) LIKE ? ESCAPE '\'))`)
		args = append(args, likeArg(q.App), likeArg(q.App))
	}
	if q.Env != "" {
		where = append(where, `d.env LIKE ? ESCAPE '\'`)
		args = append(args, likeArg(q.Env))
	}
	if q.Node != "" {
		where = append(where, `(d.node LIKE ? ESCAPE '\' OR (d.kind = 'node' AND lower(d.name) LIKE ? ESCAPE '\'))`)
		args = append(args, likeArg(q.Node), likeArg(q.Node))
	}
	if q.Tag != "" {
		where = append(where, `d.tags LIKE ? ESCAPE '\'`)
		args = append(args, "%|"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(q.Tag))+"|%")
	}
	if kind != "" {
		where = append(where, `d.kind = ?`)
		args = append(args, string(kind))
	}
	query := sel + from
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += order + fmt.Sprintf(` LIMIT %d`, candidateLimit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []candidate
	for rows.Next() {
		var (
			c       candidate
			kindStr string
			tags    string
			confirm int
			updated int64
			bm      float64
		)
		d := &c.doc
		if err := rows.Scan(&d.Ref, &kindStr, &d.Title, &d.Body, &tags, &d.App, &d.Env, &d.Node, &d.Prefix, &d.Name,
			&d.Ability, &confirm, &d.Href, &d.ActionID, &updated, &bm); err != nil {
			return nil, fmt.Errorf("search: scan: %w", err)
		}
		d.Kind = Kind(kindStr)
		d.Confirm = confirm != 0
		d.UpdatedAt = time.Unix(updated, 0)
		d.Tags = splitTags(tags)
		c.relevance = 1
		if len(long) > 0 {
			c.relevance = max(-bm, 0.01)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: rows: %w", err)
	}
	return out, nil
}

type matchMode int

const (
	matchStrict matchMode = iota
	matchLoose
)

func splitTags(col string) []string {
	col = strings.Trim(col, "|")
	if col == "" {
		return nil
	}
	return strings.Split(col, "|")
}

// trigramPhrases splits terms into overlapping 3-rune windows so a typo still
// shares most of its trigrams with the intended word.
func trigramPhrases(terms []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range terms {
		r := []rune(strings.ToLower(t))
		for i := 0; i+3 <= len(r) && len(out) < 30; i++ {
			g := string(r[i : i+3])
			if _, dup := seen[g]; dup {
				continue
			}
			seen[g] = struct{}{}
			out = append(out, ftsPhrase(g))
		}
	}
	return out
}
