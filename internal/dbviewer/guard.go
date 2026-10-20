// Package dbviewer runs read-only (and, when explicitly allowed, write)
// SQL against a managed database by exec'ing the engine's own client
// inside its container. Credentials never leave the container.
package dbviewer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Dialect selects the lexing and deny-list rules for one SQL family.
type Dialect string

// Supported dialects.
const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
	DialectMariaDB  Dialect = "mariadb"
)

// Statement kinds the guard classifies an accepted statement into.
const (
	KindQuery   = "query"
	KindExplain = "explain"
	KindShow    = "show"
	KindWrite   = "write"
)

// Rejection reasons, exported so handlers can map them to a 400.
var (
	ErrEmpty             = errors.New("statement is empty")
	ErrMultiStatement    = errors.New("only a single statement is allowed")
	ErrBackslash         = errors.New("backslash is not allowed in console statements")
	ErrControlChar       = errors.New("statement contains a control character")
	ErrUnterminated      = errors.New("unterminated string, identifier, or comment")
	ErrExecutableComment = errors.New("executable comments are not allowed")
	ErrUnicodeEscape     = errors.New("unicode escape syntax is not allowed")
	ErrNotAllowed        = errors.New("statement type is not allowed in read-only mode")
	ErrDenied            = errors.New("statement uses a function or keyword that is not allowed")
)

// Statement is a statement the guard accepted.
type Statement struct {
	// SQL is the statement with its trailing semicolon removed.
	SQL         string
	Kind        string
	Fingerprint string
	// First is the lowercased leading keyword.
	First string
}

type tokKind int

const (
	tokWord tokKind = iota
	tokIdent
	tokString
	tokNumber
	tokPunct
)

type token struct {
	kind  tokKind
	text  string
	start int
}

var readFirst = map[string]bool{
	"select": true, "with": true, "table": true, "values": true,
	"show": true, "explain": true, "describe": true, "desc": true,
}

var writeFirst = map[string]bool{
	"insert": true, "update": true, "delete": true, "merge": true, "replace": true,
	"create": true, "alter": true, "drop": true, "truncate": true, "grant": true,
	"revoke": true, "comment": true, "rename": true, "reindex": true,
}

// readDeniedWords are rejected anywhere in a read-only statement, not just
// at the start: data-modifying CTEs, SELECT INTO, FOR UPDATE.
var readDeniedWords = map[string]bool{
	"insert": true, "update": true, "delete": true, "merge": true,
	"into": true, "truncate": true, "outfile": true, "dumpfile": true,
}

// deniedFuncs are rejected in every mode: they read server files, run
// processes, or reach other sessions and servers.
var deniedFuncs = map[string]bool{
	"pg_terminate_backend": true, "pg_cancel_backend": true, "pg_reload_conf": true,
	"pg_rotate_logfile": true, "pg_create_restore_point": true, "pg_promote": true,
	"pg_stat_file": true, "pg_notify": true, "pg_authid": true, "pg_shadow": true,
	"pg_user_mapping": true, "pg_user_mappings": true, "pg_hba_file_rules": true,
	"pg_file_settings": true, "pg_largeobject": true, "pg_ident_file_mappings": true,
	"lo_import": true, "lo_export": true, "lo_unlink": true, "lo_create": true,
	"lo_creat": true, "lo_put": true, "lo_from_bytea": true, "lo_open": true,
	"lo_write": true, "lo_truncate": true, "lo_get": true,
	"load_file": true, "benchmark": true, "get_lock": true, "release_lock": true,
	"release_all_locks": true, "sys_exec": true, "sys_eval": true,
	"master_pos_wait": true, "source_pos_wait": true,
}

var deniedFuncPrefixes = []string{
	"pg_read_", "pg_ls_", "pg_file_", "pg_backup_", "pg_switch_", "pg_advisory",
	"pg_logical", "pg_replication", "pg_wal_replay", "pg_drop_replication",
	"dblink", "query_to_xml", "pg_start_backup", "pg_stop_backup", "pg_import_system",
}

// readOnlyDeniedFuncs add the sequence and config mutators, which a
// read-only transaction already blocks; denied here too for a clear error.
var readOnlyDeniedFuncs = map[string]bool{"nextval": true, "setval": true, "set_config": true}

// deniedPairs are two-keyword sequences rejected even in write mode.
var deniedPairs = map[[2]string]bool{
	{"alter", "system"}: true, {"create", "extension"}: true, {"alter", "extension"}: true,
	{"create", "language"}: true, {"create", "server"}: true,
}

// Check lexes sql and decides whether it may run. A rejected statement is
// never sent to the database. This is defense in depth: the read-only
// transaction and PGOPTIONS the runner sets are the real enforcement.
func Check(d Dialect, sql string, allowWrites bool) (Statement, error) {
	if strings.ContainsRune(sql, '\\') {
		return Statement{}, ErrBackslash
	}
	for _, r := range sql {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return Statement{}, ErrControlChar
		}
	}
	toks, err := lex(d, sql)
	if err != nil {
		return Statement{}, err
	}
	if len(toks) == 0 {
		return Statement{}, ErrEmpty
	}
	body := toks
	text := sql
	for i, t := range toks {
		if t.kind == tokPunct && t.text == ";" {
			if i != len(toks)-1 {
				return Statement{}, ErrMultiStatement
			}
			body = toks[:i]
			text = sql[:t.start]
		}
	}
	if len(body) == 0 {
		return Statement{}, ErrEmpty
	}
	first := body[0]
	if first.kind != tokWord {
		return Statement{}, ErrNotAllowed
	}
	if !readFirst[first.text] && (!allowWrites || !writeFirst[first.text]) {
		return Statement{}, fmt.Errorf("%w: %s", ErrNotAllowed, first.text)
	}
	if err := scanDenied(body, allowWrites); err != nil {
		return Statement{}, err
	}
	return Statement{
		SQL:         strings.TrimSpace(text),
		Kind:        kindOf(first.text),
		Fingerprint: fingerprint(body),
		First:       first.text,
	}, nil
}

func kindOf(first string) string {
	switch first {
	case "explain", "describe", "desc":
		return KindExplain
	case "show":
		return KindShow
	case "select", "with", "table", "values":
		return KindQuery
	}
	return KindWrite
}

func scanDenied(body []token, allowWrites bool) error {
	prev := ""
	for _, t := range body {
		if t.kind != tokWord && t.kind != tokIdent {
			prev = ""
			continue
		}
		w := strings.ToLower(t.text)
		if deniedFuncs[w] {
			return fmt.Errorf("%w: %s", ErrDenied, w)
		}
		for _, p := range deniedFuncPrefixes {
			if strings.HasPrefix(w, p) {
				return fmt.Errorf("%w: %s", ErrDenied, w)
			}
		}
		if t.kind == tokWord {
			if w == "copy" || w == "program" {
				return fmt.Errorf("%w: %s", ErrDenied, w)
			}
			if deniedPairs[[2]string{prev, w}] {
				return fmt.Errorf("%w: %s %s", ErrDenied, prev, w)
			}
			if !allowWrites && (readDeniedWords[w] || readOnlyDeniedFuncs[w]) {
				return fmt.Errorf("%w: %s", ErrDenied, w)
			}
		}
		if t.kind == tokIdent && !allowWrites && readOnlyDeniedFuncs[w] {
			return fmt.Errorf("%w: %s", ErrDenied, w)
		}
		prev = w
	}
	return nil
}

// fingerprint hashes the token stream with literals collapsed, so the
// same statement shape logs the same id without logging its values.
func fingerprint(body []token) string {
	var b strings.Builder
	for _, t := range body {
		switch t.kind {
		case tokString, tokNumber:
			b.WriteString("? ")
		default:
			b.WriteString(strings.ToLower(t.text))
			b.WriteByte(' ')
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9' || c == '$'
}

func lex(d Dialect, s string) ([]token, error) {
	var toks []token
	pg := d == DialectPostgres
	i, n := 0, len(s)
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < n && s[i+1] == '-' && (pg || i+2 >= n || s[i+2] == ' ' || s[i+2] == '\t' || s[i+2] == '\n' || s[i+2] == '\r'):
			i = skipLine(s, i)
		case c == '#' && !pg:
			i = skipLine(s, i)
		case c == '/' && i+1 < n && s[i+1] == '*':
			if !pg && i+2 < n && (s[i+2] == '!' || strings.HasPrefix(s[i+2:], "M!")) {
				return nil, ErrExecutableComment
			}
			end, err := skipBlockComment(s, i, pg)
			if err != nil {
				return nil, err
			}
			i = end
		case c == '\'':
			end, err := skipQuoted(s, i, '\'')
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{tokString, s[i:end], i})
			i = end
		case c == '"' || c == '`' && !pg:
			end, err := skipQuoted(s, i, c)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{tokIdent, strings.ReplaceAll(s[i+1:end-1], string(c)+string(c), string(c)), i})
			i = end
		case c == '$' && pg && (i == 0 || !isIdentPart(s[i-1])):
			end, ok, err := skipDollar(s, i)
			if err != nil {
				return nil, err
			}
			if ok {
				toks = append(toks, token{tokString, s[i:end], i})
				i = end
			} else {
				toks = append(toks, token{tokPunct, "$", i})
				i++
			}
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentPart(s[j]) {
				j++
			}
			word := s[i:j]
			if strings.EqualFold(word, "u") && j+1 < n && s[j] == '&' && (s[j+1] == '\'' || s[j+1] == '"') {
				return nil, ErrUnicodeEscape
			}
			toks = append(toks, token{tokWord, strings.ToLower(word), i})
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < n && (isIdentPart(s[j]) || s[j] == '.') {
				j++
			}
			toks = append(toks, token{tokNumber, s[i:j], i})
			i = j
		default:
			toks = append(toks, token{tokPunct, string(c), i})
			i++
		}
	}
	return toks, nil
}

func skipLine(s string, i int) int {
	for i < len(s) && s[i] != '\n' {
		i++
	}
	return i
}

func skipBlockComment(s string, i int, nested bool) (int, error) {
	depth := 0
	for i < len(s) {
		switch {
		case s[i] == '/' && i+1 < len(s) && s[i+1] == '*' && (nested || depth == 0):
			depth++
			i += 2
		case s[i] == '*' && i+1 < len(s) && s[i+1] == '/':
			depth--
			i += 2
			if depth == 0 {
				return i, nil
			}
		default:
			i++
		}
	}
	return 0, ErrUnterminated
}

// skipQuoted returns the index just past the closing quote, treating a
// doubled quote as an escaped one.
func skipQuoted(s string, i int, q byte) (int, error) {
	i++
	for i < len(s) {
		if s[i] == q {
			if i+1 < len(s) && s[i+1] == q {
				i += 2
				continue
			}
			return i + 1, nil
		}
		i++
	}
	return 0, ErrUnterminated
}

// skipDollar scans a Postgres dollar-quoted string starting at s[i]=='$'.
// ok is false when the $ is not a quote opener (a $1 parameter).
func skipDollar(s string, i int) (end int, ok bool, err error) {
	j := i + 1
	if j < len(s) && isIdentStart(s[j]) && s[j] < 0x80 {
		for j < len(s) && isIdentPart(s[j]) && s[j] != '$' {
			j++
		}
	}
	if j >= len(s) || s[j] != '$' {
		return 0, false, nil
	}
	delim := s[i : j+1]
	idx := strings.Index(s[j+1:], delim)
	if idx < 0 {
		return 0, false, ErrUnterminated
	}
	return j + 1 + idx + len(delim), true, nil
}
