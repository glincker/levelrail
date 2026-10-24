package datamigrate

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
)

var createExtRe = regexp.MustCompile(`^CREATE EXTENSION (?:IF NOT EXISTS )?"?([A-Za-z0-9_\-]+)"?(?: WITH SCHEMA "?([A-Za-z0-9_\-]+)"?)?`)

var createSchemaRe = regexp.MustCompile(`^CREATE SCHEMA (?:IF NOT EXISTS )?("?[A-Za-z0-9_\-]+"?);`)

// dumpFilter rewrites a plain-SQL pg_dump stream so it restores without the
// provider's superuser powers, and records what it changed. COPY data is
// passed through untouched.
type dumpFilter struct {
	mu       sync.Mutex
	findings []PreflightCheck
	seen     map[string]bool
}

func (f *dumpFilter) note(id, sev, msg, next string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	if f.seen[id+msg] {
		return
	}
	f.seen[id+msg] = true
	f.findings = append(f.findings, PreflightCheck{ID: id, Severity: sev, Message: msg, NextAction: next})
}

// Findings lists what the filter changed so far.
func (f *dumpFilter) Findings() []PreflightCheck {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PreflightCheck(nil), f.findings...)
}

func (f *dumpFilter) wrap(r io.Reader) io.Reader {
	return &filterReader{f: f, br: bufio.NewReaderSize(r, 64<<10), rewrite: map[string]bool{}}
}

type filterReader struct {
	f       *dumpFilter
	br      *bufio.Reader
	out     bytes.Buffer
	err     error
	inCopy  bool
	midLine bool
	rewrite map[string]bool
}

func (r *filterReader) Read(p []byte) (int, error) {
	for r.out.Len() == 0 && r.err == nil {
		chunk, err := r.br.ReadSlice('\n')
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			// A line longer than the buffer is data or a huge statement, never a rule target.
			r.out.Write(chunk)
			r.midLine = true
		case len(chunk) > 0 && r.midLine:
			r.out.Write(chunk)
			r.midLine, r.err = false, err
		case len(chunk) > 0:
			r.line(string(chunk))
			r.err = err
		default:
			r.err = err
		}
	}
	if r.out.Len() > 0 {
		return r.out.Read(p)
	}
	return 0, r.err
}

func (r *filterReader) line(l string) {
	if r.inCopy {
		if strings.TrimRight(l, "\r\n") == `\.` {
			r.inCopy = false
		}
		r.out.WriteString(l)
		return
	}
	if m := createSchemaRe.FindStringSubmatch(l); m != nil {
		// The restore only resets public, so a schema left by an earlier run
		// would make this CREATE fail. Dropping it here keeps one transaction.
		if name := strings.Trim(m[1], `"`); name != "public" && name != "information_schema" && !strings.HasPrefix(name, "pg_") {
			r.out.WriteString("DROP SCHEMA IF EXISTS " + m[1] + " CASCADE;\n")
		}
	}
	switch {
	case strings.HasPrefix(l, "COMMENT ON EXTENSION "):
		r.f.note("extension_comment", SeverityWarn, "Extension comments were skipped: only the extension owner may set them.", "")
		return
	case strings.HasPrefix(l, "CREATE EXTENSION "):
		if m := createExtRe.FindStringSubmatch(l); m != nil {
			if providerOnlyExtensions[strings.ToLower(m[1])] {
				r.f.note("provider_extensions", SeverityWarn, fmt.Sprintf("The provider-only extension %q was skipped.", m[1]),
					"Features that depend on it will not work on the target.")
				return
			}
			if s := m[2]; s != "" && s != "public" && s != "pg_catalog" {
				r.rewrite[s] = true
				l = strings.Replace(l, `SCHEMA "`+s+`"`, "SCHEMA public", 1)
				l = strings.Replace(l, "SCHEMA "+s, "SCHEMA public", 1)
				r.f.note("extension_schema", SeverityWarn, fmt.Sprintf("Extension %q lived in the provider schema %q and now lives in public.", m[1], s),
					"Names qualified with the old schema were rewritten in the dump; check any code that calls them by that name.")
			}
		}
	}
	for s := range r.rewrite {
		l = strings.ReplaceAll(l, s+".", "public.")
	}
	if strings.HasPrefix(l, "COPY ") && strings.HasSuffix(strings.TrimRight(l, "\r\n"), " FROM stdin;") {
		r.inCopy = true
	}
	r.out.WriteString(l)
}

// filteringRestorer wraps a Restorer so the dump passes through a dumpFilter.
type filteringRestorer struct {
	inner  Restorer
	filter *dumpFilter
}

func (fr filteringRestorer) Restore(ctx context.Context, engine, container string, dump io.Reader) error {
	return fr.inner.Restore(ctx, engine, container, fr.filter.wrap(dump))
}
