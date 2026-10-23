// Package dashboards defines the portable custom dashboard document, its JSON Schema and the starter templates.
package dashboards

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Version is the document format version; export files carry it so imports can reject newer formats.
const Version = 1

// Dashboard scopes, target kinds and widget types, matching schema.json.
const (
	ScopeUser   = "user"
	ScopeShared = "shared"

	TargetApp      = "app"
	TargetDatabase = "database"
	TargetNode     = "node"

	WidgetMetricChart    = "metric_chart"
	WidgetRequestPcts    = "request_percentiles"
	WidgetErrorRate      = "error_rate"
	WidgetLogTail        = "log_tail"
	WidgetDeployTimeline = "deploy_timeline"
	WidgetResourceCounts = "resource_counts"
	WidgetCost           = "cost"
	WidgetAlertStatus    = "alert_status"
	WidgetNodeCapacity   = "node_capacity"
	WidgetLink           = "link"
)

// Target names the app, database or node a widget reads from.
type Target struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Widget is one tile on the dashboard grid.
type Widget struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Title  string  `json:"title,omitempty"`
	Size   string  `json:"size"`
	Target *Target `json:"target,omitempty"`
	Metric string  `json:"metric,omitempty"`
	Query  string  `json:"query,omitempty"`
	URL    string  `json:"url,omitempty"`
}

// Doc is the exportable body of a dashboard.
type Doc struct {
	Version        int      `json:"version"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	TimeRange      string   `json:"time_range,omitempty"`
	RefreshSeconds int      `json:"refresh_seconds,omitempty"`
	Widgets        []Widget `json:"widgets"`
}

// allowedTargets lists the target kinds each widget type can read from; types absent here take no target.
var allowedTargets = map[string][]string{
	WidgetMetricChart:    {TargetApp, TargetDatabase, TargetNode},
	WidgetRequestPcts:    {TargetApp},
	WidgetErrorRate:      {TargetApp},
	WidgetLogTail:        {TargetApp},
	WidgetDeployTimeline: {TargetApp},
	WidgetAlertStatus:    {TargetApp},
	WidgetNodeCapacity:   {TargetNode},
}

//go:embed schema.json
var schemaJSON []byte

var (
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
)

func compiled() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
		if err != nil {
			schemaErr = fmt.Errorf("dashboards: parse schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("dashboard.json", doc); err != nil {
			schemaErr = fmt.Errorf("dashboards: add schema: %w", err)
			return
		}
		schema, schemaErr = c.Compile("dashboard.json")
	})
	return schema, schemaErr
}

// ErrInvalid wraps every validation failure so callers can answer 400.
var ErrInvalid = errors.New("invalid dashboard")

// Parse decodes and validates an exported dashboard document.
func Parse(raw []byte) (Doc, error) {
	sch, err := compiled()
	if err != nil {
		return Doc{}, err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return Doc{}, fmt.Errorf("%w: not valid JSON", ErrInvalid)
	}
	if err := sch.Validate(inst); err != nil {
		return Doc{}, fmt.Errorf("%w: %s", ErrInvalid, firstSchemaIssue(err))
	}
	var d Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		return Doc{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := d.checkWidgets(); err != nil {
		return Doc{}, err
	}
	return d, nil
}

func firstSchemaIssue(err error) string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return err.Error()
	}
	var msg string
	var walk func(u jsonschema.OutputUnit)
	walk = func(u jsonschema.OutputUnit) {
		if msg != "" {
			return
		}
		if u.Error != nil && len(u.Errors) == 0 {
			loc := strings.Trim(strings.ReplaceAll(u.InstanceLocation, "/", "."), ".")
			if loc == "" {
				loc = "document"
			}
			msg = loc + ": " + u.Error.String()
			return
		}
		for _, c := range u.Errors {
			walk(c)
		}
	}
	walk(*ve.BasicOutput())
	if msg == "" {
		return "does not match the dashboard schema"
	}
	return msg
}

// Validate checks an in-memory document the same way Parse checks a file.
func (d Doc) Validate() error {
	raw, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("dashboards: encode: %w", err)
	}
	_, err = Parse(raw)
	return err
}

func (d Doc) checkWidgets() error {
	seen := make(map[string]bool, len(d.Widgets))
	for i, w := range d.Widgets {
		if seen[w.ID] {
			return fmt.Errorf("%w: widgets.%d: duplicate id %q", ErrInvalid, i, w.ID)
		}
		seen[w.ID] = true
		if w.Type == WidgetLink {
			if err := checkLink(w); err != nil {
				return fmt.Errorf("%w: widgets.%d: %w", ErrInvalid, i, err)
			}
			continue
		}
		kinds, needs := allowedTargets[w.Type]
		if !needs {
			continue
		}
		if w.Target == nil {
			return fmt.Errorf("%w: widgets.%d: %s needs a target", ErrInvalid, i, w.Type)
		}
		allowed := false
		for _, k := range kinds {
			allowed = allowed || k == w.Target.Kind
		}
		if !allowed {
			return fmt.Errorf("%w: widgets.%d: %s cannot read from a %s", ErrInvalid, i, w.Type, w.Target.Kind)
		}
		if w.Type == WidgetMetricChart && w.Metric == "" {
			return fmt.Errorf("%w: widgets.%d: metric_chart needs a metric", ErrInvalid, i)
		}
	}
	return nil
}

// checkLink keeps a link widget to a titled http or https address, so a shared dashboard cannot carry script or data URLs.
func checkLink(w Widget) error {
	if strings.TrimSpace(w.Title) == "" {
		return errors.New("link needs a title")
	}
	u, err := url.Parse(w.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("link needs an http or https address")
	}
	return nil
}
