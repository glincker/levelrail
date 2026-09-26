package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// DiagnosisChange is one field edit a fix proposes, on the app resource's
// own JSON paths ("port", "health.readiness.path", "resources.memory_bytes",
// "env.NAME").
type DiagnosisChange struct {
	Field      string `json:"field"`
	From       string `json:"from"`
	To         string `json:"to"`
	NeedsInput bool   `json:"needs_input,omitempty"`
}

// DiagnosisFix is one remedy for a cause. Kind is "patch", "input" or
// "manual"; only the first two carry changes.
type DiagnosisFix struct {
	N        int               `json:"n"`
	Label    string            `json:"label"`
	Kind     string            `json:"kind"`
	Changes  []DiagnosisChange `json:"changes"`
	Hint     string            `json:"hint,omitempty"`
	Redeploy bool              `json:"redeploy,omitempty"`
}

// DiagnosisCause is one typed, evidence-backed reason for a failure.
type DiagnosisCause struct {
	Code        string            `json:"code"`
	Title       string            `json:"title"`
	Explanation string            `json:"explanation"`
	Confidence  string            `json:"confidence"`
	Evidence    []DiagnosisSignal `json:"evidence"`
	Fixes       []DiagnosisFix    `json:"fixes"`
}

// FindDiagnosisFix returns fix number n across every cause.
func FindDiagnosisFix(d DiagnosisResource, n int) (DiagnosisFix, bool) {
	for _, c := range d.Causes {
		for _, f := range c.Fixes {
			if f.N == n {
				return f, true
			}
		}
	}
	return DiagnosisFix{}, false
}

// ErrFixStale means the app no longer holds the value the fix was computed
// from, so applying it could overwrite a newer edit.
var ErrFixStale = errors.New("app changed since the diagnosis, run it again")

// PreflightCheck is one preflight result.
type PreflightCheck struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Fix    string `json:"fix,omitempty"`
}

// PreflightReport mirrors internal/preflight.Report.
type PreflightReport struct {
	Status string           `json:"status"`
	Checks []PreflightCheck `json:"checks"`
}

// PreflightApp calls POST /api/v1/apps/{name}/preflight. requiredEnv names
// variables the app must have set, since that set is not stored.
func (c *Client) PreflightApp(ctx context.Context, name string, requiredEnv []string) (PreflightReport, error) {
	var out PreflightReport
	body := map[string][]string{"required_env": requiredEnv}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/preflight", body, &out)
	return out, err
}

// ApplyDiagnosisFix applies a patch or input fix to the app through the
// ordinary GET then PUT path, so the caller's own permissions and the audit
// log apply. inputs supplies values for changes marked NeedsInput, keyed by
// field. It returns the fields changed. The app is round-tripped as raw JSON
// so fields this client does not model are never dropped.
func (c *Client) ApplyDiagnosisFix(ctx context.Context, name string, fix DiagnosisFix, inputs map[string]string, redeploy bool) ([]string, error) {
	if fix.Kind == "manual" || len(fix.Changes) == 0 {
		return nil, errors.New("this fix is manual: " + fix.Hint)
	}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name), nil, &raw); err != nil {
		return nil, fmt.Errorf("apiclient: load app for fix: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var app map[string]any
	if err := dec.Decode(&app); err != nil {
		return nil, fmt.Errorf("apiclient: decode app for fix: %w", err)
	}
	fields, err := PatchAppMap(app, fix.Changes, inputs)
	if err != nil {
		return nil, err
	}
	var saved json.RawMessage
	if err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name), app, &saved); err != nil {
		return nil, fmt.Errorf("apiclient: save app fix: %w", err)
	}
	if redeploy && fix.Redeploy {
		image, _ := app["image"].(string)
		if _, err := c.DeployApp(ctx, name, image, false); err != nil {
			return fields, fmt.Errorf("apiclient: fix applied but redeploy failed: %w", err)
		}
	}
	return fields, nil
}

// PatchAppMap applies changes to a decoded app resource (numbers as
// json.Number). It refuses a change whose current value differs from From.
func PatchAppMap(app map[string]any, changes []DiagnosisChange, inputs map[string]string) ([]string, error) {
	var fields []string
	for _, ch := range changes {
		to := ch.To
		if ch.NeedsInput {
			v, ok := inputs[ch.Field]
			if !ok || v == "" {
				return nil, fmt.Errorf("a value is required for %s", ch.Field)
			}
			to = v
		}
		if err := patchField(app, ch, to); err != nil {
			return nil, err
		}
		fields = append(fields, ch.Field)
	}
	return fields, nil
}

func patchField(app map[string]any, ch DiagnosisChange, to string) error {
	switch {
	case ch.Field == "port":
		return setNumber(app, "port", ch.From, to)
	case ch.Field == "resources.memory_bytes":
		res, _ := app["resources"].(map[string]any)
		if res == nil {
			return fmt.Errorf("%w: no resources block", ErrFixStale)
		}
		return setNumber(res, "memory_bytes", ch.From, to)
	case ch.Field == "health.readiness.path":
		probe, ok := nested(app, "health", "readiness")
		if !ok {
			return fmt.Errorf("%w: no readiness probe configured", ErrFixStale)
		}
		if cur, _ := probe["path"].(string); cur != ch.From {
			return fmt.Errorf("%w: readiness path is %q", ErrFixStale, cur)
		}
		probe["path"] = to
		return nil
	case strings.HasPrefix(ch.Field, "env."):
		env, _ := app["env"].(map[string]any)
		if env == nil {
			env = map[string]any{}
			app["env"] = env
		}
		key := strings.TrimPrefix(ch.Field, "env.")
		if cur, _ := env[key].(string); cur != ch.From {
			return fmt.Errorf("%w: %s changed since the diagnosis", ErrFixStale, key)
		}
		env[key] = to
		return nil
	}
	return fmt.Errorf("unsupported fix field %q", ch.Field)
}

func nested(m map[string]any, keys ...string) (map[string]any, bool) {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func setNumber(m map[string]any, key, from, to string) error {
	if from != "" {
		cur, _ := m[key].(json.Number)
		if cur.String() != from {
			return fmt.Errorf("%w: %s is %s", ErrFixStale, key, cur.String())
		}
	}
	if _, err := strconv.ParseInt(to, 10, 64); err != nil {
		return fmt.Errorf("invalid number %q for %s", to, key)
	}
	m[key] = json.Number(to)
	return nil
}
