package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Limits on what a plugin can make the dashboard render.
const (
	maxViewRows     = 200
	maxViewItems    = 100
	maxViewPoints   = 100
	maxCellBytes    = 200
	maxItemBytes    = 300
	maxMarkdown     = 8 << 10
	maxStatBytes    = 80
	maxJSONNesting  = 32
	StatusOK        = "ok"
	StatusWarning   = "warning"
	StatusError     = "error"
	StatusUnknown   = "unknown"
	widgetBindError = "the plugin's data did not have this field"
)

// PanelView is a panel's data after the host bound it to the declared widgets.
// Every value is plain text or a number: the dashboard never receives raw plugin JSON.
type PanelView struct {
	Widgets []WidgetView `json:"widgets"`
}

// ChartPoint is one labelled value of a chart widget.
type ChartPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// WidgetView is one bound widget. Error is set when the plugin's data did not
// match the manifest, so one bad widget does not blank the whole panel.
type WidgetView struct {
	Type    string       `json:"type"`
	Title   string       `json:"title"`
	Error   string       `json:"error,omitempty"`
	Value   string       `json:"value,omitempty"`
	Unit    string       `json:"unit,omitempty"`
	State   string       `json:"state,omitempty"`
	Text    string       `json:"text,omitempty"`
	Columns []Column     `json:"columns,omitempty"`
	Rows    [][]string   `json:"rows,omitempty"`
	Items   []string     `json:"items,omitempty"`
	Points  []ChartPoint `json:"points,omitempty"`
	Action  string       `json:"action,omitempty"`
	Label   string       `json:"label,omitempty"`
	Confirm string       `json:"confirm,omitempty"`
}

// BindPanel validates data against the panel's widgets and returns the view.
// It returns an error only when data is not a JSON object.
func BindPanel(p Panel, data []byte) (PanelView, error) {
	root, err := decodeObject(data)
	if err != nil {
		return PanelView{}, fmt.Errorf("%w: not a JSON object", ErrBadData)
	}
	if jsonDepth(data) > maxJSONNesting {
		return PanelView{}, fmt.Errorf("%w: nested too deeply", ErrBadData)
	}
	view := PanelView{Widgets: make([]WidgetView, 0, len(p.Widgets))}
	for _, w := range p.Widgets {
		view.Widgets = append(view.Widgets, bindWidget(w, root))
	}
	return view, nil
}

func bindWidget(w Widget, root map[string]json.RawMessage) WidgetView {
	v := WidgetView{Type: w.Type, Title: w.Title, Unit: w.Unit}
	if w.Type == WidgetButton {
		v.Action, v.Label, v.Confirm = w.Action, w.Label, w.Confirm
		return v
	}
	raw, ok := lookup(root, w.Field)
	if !ok {
		v.Error = widgetBindError
		return v
	}
	switch w.Type {
	case WidgetStat:
		v.Value = clip(scalar(raw), maxStatBytes)
	case WidgetMarkdown:
		v.Text = SanitizeMarkdown(scalar(raw))
	case WidgetStatus:
		v.State, v.Text = bindStatus(raw)
	case WidgetList:
		v.Items = bindList(raw)
	case WidgetTable:
		v.Columns = w.Columns
		v.Rows = bindTable(raw, w.Columns)
	case WidgetChart:
		v.Points = bindChart(raw, w.LabelKey, w.ValueKey)
	}
	return v
}

func bindStatus(raw json.RawMessage) (string, string) {
	state, text := scalar(raw), ""
	if obj, err := decodeObject(raw); err == nil {
		state, text = scalar(obj["state"]), scalar(obj["text"])
	}
	switch s := strings.ToLower(strings.TrimSpace(state)); s {
	case StatusOK, StatusWarning, StatusError:
		return s, clip(stripControl(text), maxItemBytes)
	default:
		return StatusUnknown, clip(stripControl(text), maxItemBytes)
	}
}

func bindList(raw json.RawMessage) []string {
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return []string{}
	}
	out := make([]string, 0, min(len(arr), maxViewItems))
	for _, el := range arr {
		if len(out) == maxViewItems {
			break
		}
		out = append(out, clip(scalar(el), maxItemBytes))
	}
	return out
}

func bindTable(raw json.RawMessage, cols []Column) [][]string {
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return [][]string{}
	}
	out := make([][]string, 0, min(len(arr), maxViewRows))
	for _, el := range arr {
		if len(out) == maxViewRows {
			break
		}
		obj, err := decodeObject(el)
		if err != nil {
			continue
		}
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = clip(scalar(obj[c.Key]), maxCellBytes)
		}
		out = append(out, row)
	}
	return out
}

func bindChart(raw json.RawMessage, labelKey, valueKey string) []ChartPoint {
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return []ChartPoint{}
	}
	out := make([]ChartPoint, 0, min(len(arr), maxViewPoints))
	for _, el := range arr {
		if len(out) == maxViewPoints {
			break
		}
		obj, err := decodeObject(el)
		if err != nil {
			continue
		}
		f, err := strconv.ParseFloat(scalar(obj[valueKey]), 64)
		if err != nil || math.IsNaN(f) || f > 1e15 || f < -1e15 {
			continue
		}
		out = append(out, ChartPoint{Label: clip(scalar(obj[labelKey]), 60), Value: f})
	}
	return out
}

func lookup(root map[string]json.RawMessage, path string) (json.RawMessage, bool) {
	cur := root
	parts := strings.Split(path, ".")
	for i, part := range parts {
		raw, ok := cur[part]
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return raw, true
		}
		next, err := decodeObject(raw)
		if err != nil {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, errNotObject
	}
	return m, nil
}

var errNotObject = errors.New("not a JSON object")

// ErrBadData marks plugin data that does not fit the panel it feeds.
var ErrBadData = errors.New("plugin: returned data that does not fit the panel")

// scalar renders a JSON string, number or bool as text. Objects, arrays and
// null become "" so structure never leaks into a cell.
func scalar(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return ""
		}
		return stripControl(s)
	case 't', 'f':
		return string(raw)
	case '{', '[', 'n':
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var n json.Number
	if dec.Decode(&n) != nil {
		return ""
	}
	return n.String()
}

func jsonDepth(b []byte) int {
	depth, deepest := 0, 0
	inStr, esc := false, false
	for _, c := range b {
		switch {
		case esc:
			esc = false
		case inStr:
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
		case c == '"':
			inStr = true
		case c == '{' || c == '[':
			depth++
			deepest = max(deepest, depth)
		case c == '}' || c == ']':
			depth--
		}
	}
	return deepest
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == '\u202e' || r == '\u2066' || r == '\u2067' || r == '\u2068' || r == '\u2069' || r == '\u202a' || r == '\u202b' || r == '\u202c' || r == '\u202d' {
			return -1
		}
		return r
	}, s)
}

var (
	htmlTagRe   = regexp.MustCompile(`<[^>]*>?`)
	mdImageRe   = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdLinkRe    = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]*)\)`)
	refDefRe    = regexp.MustCompile(`(?m)^\s*\[[^\]]+\]:\s*\S+.*$`)
	entityRe    = regexp.MustCompile(`&#?[A-Za-z0-9]+;`)
	autolinkRe  = regexp.MustCompile(`(?i)\b(?:javascript|data|vbscript):`)
	safeLinkURL = regexp.MustCompile(`^https?://[^\s<>"'\x60]+$`)
)

// SanitizeMarkdown reduces plugin text to a safe markdown subset: raw HTML, HTML
// entities, images, reference links and non-http(s) links are removed. The
// dashboard renders what remains with its own minimal renderer, never as HTML.
func SanitizeMarkdown(s string) string {
	s = clip(stripControl(s), maxMarkdown)
	for range 4 {
		next := htmlTagRe.ReplaceAllString(s, "")
		if next == s {
			break
		}
		s = next
	}
	s = entityRe.ReplaceAllString(s, "")
	s = refDefRe.ReplaceAllString(s, "")
	s = mdImageRe.ReplaceAllString(s, "$1")
	s = mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := mdLinkRe.FindStringSubmatch(m)
		if safeLinkURL.MatchString(parts[2]) {
			return m
		}
		return parts[1]
	})
	return autolinkRe.ReplaceAllString(s, "")
}
