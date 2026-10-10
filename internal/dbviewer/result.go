package dbviewer

import (
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Result is one statement's output. A nil cell is SQL NULL.
type Result struct {
	Columns    []string    `json:"columns"`
	Rows       [][]*string `json:"rows"`
	RowCount   int         `json:"row_count"`
	Truncated  bool        `json:"truncated"`
	DurationMs int64       `json:"duration_ms"`
}

// Execer is the slice of docker.Runtime the console needs.
type Execer interface {
	ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error)
}

// Target is one running database container.
type Target struct {
	Exec        Execer
	ContainerID string
	Dialect     Dialect
	Limits      Limits
}

// ErrTimeout reports that the statement ran past the configured timeout.
var ErrTimeout = errors.New("query exceeded the configured timeout")

// QueryError carries the database's own error text, which is safe to show:
// it describes the caller's statement and never contains credentials.
type QueryError struct{ Message string }

func (e *QueryError) Error() string { return e.Message }

var errByteCap = errors.New("result size cap reached")

type capReader struct {
	r     io.Reader
	left  int
	tripd bool
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		c.tripd = true
		return 0, errByteCap
	}
	if len(p) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= n
	return n, err
}

func truncateCell(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func parseCSV(r io.Reader, sentinel string, lim Limits) (Result, error) {
	capped := &capReader{r: r, left: lim.MaxBytes}
	cr := csv.NewReader(capped)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	res := Result{Columns: []string{}, Rows: [][]*string{}}
	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return res, nil
	}
	if err != nil {
		return res, csvErr(err, capped, &res)
	}
	res.Columns = header
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return res, csvErr(err, capped, &res)
		}
		if len(res.Rows) >= lim.MaxRows {
			res.Truncated = true
			break
		}
		row := make([]*string, len(rec))
		for i, cell := range rec {
			if cell == sentinel {
				continue
			}
			v := truncateCell(cell, lim.MaxCellBytes)
			row[i] = &v
		}
		res.Rows = append(res.Rows, row)
	}
	res.RowCount = len(res.Rows)
	return res, nil
}

func csvErr(err error, capped *capReader, res *Result) error {
	if capped.tripd || errors.Is(err, errByteCap) {
		res.Truncated = true
		res.RowCount = len(res.Rows)
		return nil
	}
	var pe *csv.ParseError
	if errors.As(err, &pe) && errors.Is(pe.Err, errByteCap) {
		res.Truncated = true
		res.RowCount = len(res.Rows)
		return nil
	}
	return err
}

type xmlField struct {
	name string
	val  *string
}

func parseXML(r io.Reader, lim Limits) (Result, error) {
	capped := &capReader{r: r, left: lim.MaxBytes}
	dec := xml.NewDecoder(capped)
	dec.Strict = false
	res := Result{Columns: []string{}, Rows: [][]*string{}}

	var row []xmlField
	var cur *xmlField
	var text strings.Builder
	inRow := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if capped.tripd {
				res.Truncated = true
				break
			}
			return res, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				inRow = true
				row = row[:0]
			case "field":
				if !inRow {
					continue
				}
				f := xmlField{}
				isNull := false
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "name":
						f.name = a.Value
					case "nil":
						isNull = a.Value == "true"
					}
				}
				if !isNull {
					empty := ""
					f.val = &empty
				}
				cur = &f
				text.Reset()
			}
		case xml.CharData:
			if cur != nil {
				text.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "field":
				if cur != nil {
					if cur.val != nil {
						v := truncateCell(text.String(), lim.MaxCellBytes)
						cur.val = &v
					}
					row = append(row, *cur)
					cur = nil
				}
			case "row":
				inRow = false
				if len(res.Rows) >= lim.MaxRows {
					res.Truncated = true
					res.RowCount = len(res.Rows)
					return res, nil
				}
				if len(res.Columns) == 0 {
					for _, f := range row {
						res.Columns = append(res.Columns, f.name)
					}
				}
				cells := make([]*string, len(row))
				for i, f := range row {
					cells[i] = f.val
				}
				res.Rows = append(res.Rows, cells)
			}
		}
	}
	res.RowCount = len(res.Rows)
	return res, nil
}

// wrapExecError turns an exec failure into a QueryError carrying only the
// database's own error lines.
func wrapExecError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrTimeout
	}
	var ee *docker.ExecExitError
	if errors.As(err, &ee) {
		return &QueryError{Message: dbErrorText(ee.Stderr, ee.ExitCode)}
	}
	return fmt.Errorf("dbviewer: exec: %w", err)
}

func dbErrorText(stderr string, exit int) string {
	var keep []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ERROR"), strings.HasPrefix(line, "psql:"), strings.HasPrefix(line, "FATAL"):
			if i := strings.Index(line, "ERROR:"); i >= 0 {
				line = line[i:]
			}
			keep = append(keep, line)
		case len(keep) > 0 && (strings.HasPrefix(line, "LINE ") || strings.HasPrefix(line, "DETAIL:") || strings.HasPrefix(line, "HINT:")):
			keep = append(keep, line)
		}
	}
	msg := strings.Join(keep, "\n")
	if msg == "" {
		return fmt.Sprintf("statement failed (exit %d)", exit)
	}
	if len(msg) > 600 {
		msg = truncateCell(msg, 600)
	}
	return msg
}
