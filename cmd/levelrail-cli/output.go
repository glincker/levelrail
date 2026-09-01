package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jmespath/go-jmespath"
)

// Exit codes. Distinct on purpose (per the CLI's own design brief: "real,
// distinct exit codes... with the actual error message on stderr either
// way"), so a script or an AI agent driving this CLI can branch on
// *why* a call failed without parsing stderr text.
const (
	exitOK         = 0
	exitUsage      = 1 // unknown command, missing positional argument, bad flag syntax
	exitValidation = 2 // well-formed flags/file, but the request they describe is invalid
	exitNetwork    = 3 // could not reach the API at all (connection refused, DNS, timeout)
	exitAPIError   = 4 // the API was reached and returned a non-2xx response
)

// exitCodeForError classifies err into one of the exit codes above.
// errors.As unwraps through fmt.Errorf("...: %w", ...) wrapping, so a
// *apiError still classifies correctly however many layers of context
// client.go's own do() and its callers added on the way up.
func exitCodeForError(err error) int {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return exitAPIError
	}
	var valErr *validationError
	if errors.As(err, &valErr) {
		return exitValidation
	}
	return exitNetwork
}

// validationError marks a failure as "the request itself is invalid"
// (a bad flag combination, an app.yaml this CLI can't yet turn into a
// request), as opposed to a network or API failure: a usage mistake vs.
// something failing downstream.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func newValidationError(format string, args ...any) error {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

// writeJSONValue marshals v as indented JSON to out, the CLI's --json
// mode contract: this is the only thing written to stdout in that mode.
func writeJSONValue(out io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

// writeJSONError writes {"error": "..."} to out: --json mode's error
// shape, deliberately the same {"error": "..."} field name
// internal/api/respond.go's own apiError already uses, so a caller
// parsing this CLI's JSON output and the control plane's own JSON error
// responses can use one code path for both.
func writeJSONError(out io.Writer, err error) error {
	return writeJSONValue(out, map[string]string{"error": err.Error()})
}

// outputFormat is the resolved --output value.
type outputFormat string

const (
	outputTable outputFormat = "table"
	outputJSON  outputFormat = "json"
	outputText  outputFormat = "text"
)

// resolveOutputFormat reconciles the old --json boolean with the newer
// --output flag: --json is shorthand for --output json, kept so scripts
// written against it before --output existed still work unchanged. An
// --output that contradicts an explicit --json is a usage error rather
// than a silent pick between the two.
func resolveOutputFormat(jsonOut bool, outputFlag string) (outputFormat, error) {
	switch outputFlag {
	case "":
		if jsonOut {
			return outputJSON, nil
		}
		return outputTable, nil
	case string(outputJSON), string(outputTable), string(outputText):
		if jsonOut && outputFlag != string(outputJSON) {
			return "", newValidationError("--json conflicts with --output %s, use one or the other", outputFlag)
		}
		return outputFormat(outputFlag), nil
	default:
		return "", newValidationError("invalid --output %q, want json, table, or text", outputFlag)
	}
}

// toGenericJSON round-trips v through encoding/json so it becomes the
// map[string]any/[]any/float64/... shape jmespath.Search and the
// generic text/table renderers below operate on, rather than v's
// original Go struct type.
func toGenericJSON(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode value: %w", err)
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, fmt.Errorf("decode value: %w", err)
	}
	return generic, nil
}

// applyQuery runs a JMESPath expression against v, AWS CLI's --query
// feature: lets a caller project or filter a response (e.g.
// "[?status=='running'].name") without piping through jq.
func applyQuery(query string, v any) (any, error) {
	generic, err := toGenericJSON(v)
	if err != nil {
		return nil, err
	}
	result, err := jmespath.Search(query, generic)
	if err != nil {
		return nil, newValidationError("--query %q: %s", query, err)
	}
	return result, nil
}

// renderResult prints data according to format/query. renderTable is
// only called on the plain default path (table format, no --query), so
// every existing command's hand-written table printer keeps producing
// exactly its current output when neither new flag is used; --query and
// --output text/json instead go through the generic renderers below,
// which is what lets every command gain both flags from this one
// function instead of each command implementing its own projection.
func renderResult(stdout io.Writer, format outputFormat, query string, data any, renderTable func()) error {
	value := data
	switch {
	case query != "":
		projected, err := applyQuery(query, data)
		if err != nil {
			return err
		}
		value = projected
	case format == outputText:
		generic, err := toGenericJSON(data)
		if err != nil {
			return err
		}
		value = generic
	}

	switch {
	case format == outputTable && query == "":
		renderTable()
		return nil
	case format == outputTable:
		return writeQueriedTable(stdout, value)
	case format == outputText:
		return writeTextValue(stdout, value)
	default:
		return writeJSONValue(stdout, value)
	}
}

// writeTextValue prints v in --output text's shape: one line per row, tab
// separated fields, no headers or alignment, meant for further piping
// through awk/cut. This is AWS CLI's own distinction between --output
// table (boxed, human-oriented) and --output text (raw, script-oriented):
// a list prints one row per element, anything else prints as a single row.
func writeTextValue(out io.Writer, v any) error {
	rows, ok := v.([]any)
	if !ok {
		rows = []any{v}
	}
	for _, row := range rows {
		var fields []string
		flattenText(row, &fields)
		if _, err := fmt.Fprintln(out, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}
	return nil
}

// flattenText appends v's scalar leaves to out in depth-first order:
// object values by sorted key (for deterministic output), array elements
// in order, so a nested JSON value becomes one flat tab-separated row.
func flattenText(v any, out *[]string) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flattenText(t[k], out)
		}
	case []any:
		for _, item := range t {
			flattenText(item, out)
		}
	case nil:
		*out = append(*out, "None")
	default:
		*out = append(*out, scalarText(t))
	}
}

// scalarText renders one JSON scalar (as produced by encoding/json's
// decode-into-any: string, bool, float64, or nil) the way --output
// text/table print it: no surrounding quotes, integers without a
// trailing ".0".
func scalarText(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case map[string]any, []any:
		var fields []string
		flattenText(t, &fields)
		return strings.Join(fields, ",")
	default:
		return fmt.Sprintf("%v", t)
	}
}

// writeQueriedTable renders a --query result as a table when it is a
// uniform, non-empty list of objects (one row per element, columns from
// the shared keys); any other shape (a scalar, a single object, a
// ragged list) isn't table-shaped, so it falls back to JSON rather than
// guessing a layout, matching AWS CLI's own caveat that --output table
// needs table-shaped data.
func writeQueriedTable(out io.Writer, v any) error {
	rows, ok := v.([]any)
	if !ok || len(rows) == 0 {
		return writeJSONValue(out, v)
	}
	cols, ok := tableColumns(rows)
	if !ok {
		return writeJSONValue(out, v)
	}

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = strings.ToUpper(c)
	}
	_, _ = fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		// tableColumns already verified every row is a map[string]any.
		obj, _ := r.(map[string]any)
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = scalarText(obj[c])
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

// tableColumns returns rows' shared, sorted column names, or false if
// rows isn't a uniform list of objects (any element not a
// map[string]any, or with a different key count than the first).
func tableColumns(rows []any) ([]string, bool) {
	first, ok := rows[0].(map[string]any)
	if !ok {
		return nil, false
	}
	cols := make([]string, 0, len(first))
	for k := range first {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	for _, r := range rows[1:] {
		obj, ok := r.(map[string]any)
		if !ok || len(obj) != len(cols) {
			return nil, false
		}
	}
	return cols, true
}

// printAppHuman prints one app resource in a human-readable, non-JSON
// form.
func printAppHuman(out io.Writer, a appResource) {
	_, _ = fmt.Fprintf(out, "name:     %s\n", a.Name)
	_, _ = fmt.Fprintf(out, "image:    %s\n", a.Image)
	_, _ = fmt.Fprintf(out, "port:     %d\n", a.Port)
	if a.HostPort != nil {
		_, _ = fmt.Fprintf(out, "host port: %d (pinned)\n", *a.HostPort)
	}
	if len(a.Domains) > 0 {
		_, _ = fmt.Fprintf(out, "domains:  %v\n", a.Domains)
	}
	if a.NodeID != "" {
		_, _ = fmt.Fprintf(out, "node:     %s\n", a.NodeID)
	}
	if a.EnvDirty {
		_, _ = fmt.Fprintln(out, "env:      pending restart (env vars saved since the running container was last recreated)")
	}
}

// printAppsTable prints a compact, aligned table of apps: list output's
// human-readable form.
func printAppsTable(out io.Writer, apps []appResource) {
	if len(apps) == 0 {
		_, _ = fmt.Fprintln(out, "no apps")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tIMAGE\tPORT\tNODE\tENV")
	for _, a := range apps {
		node := a.NodeID
		if node == "" {
			node = "(local)"
		}
		env := "-"
		if a.EnvDirty {
			env = "pending restart"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", a.Name, a.Image, a.Port, node, env)
	}
	_ = tw.Flush()
}

// printConditionsHuman prints an app's or database's current reconcile
// conditions ("apps status"/"databases status" output): current status,
// not a history log, matching what GetConditions itself returns.
func printConditionsHuman(out io.Writer, conditions []conditionResource) {
	if len(conditions) == 0 {
		_, _ = fmt.Fprintln(out, "no conditions reported yet")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TYPE\tSTATUS\tREASON\tMESSAGE\tLAST TRANSITION")
	for _, c := range conditions {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.Type, c.Status, c.Reason, c.Message, c.LastTransitionTime.Format(time.RFC3339))
	}
	_ = tw.Flush()
}

// printDiagnosisHuman prints "apps diagnose" output.
func printDiagnosisHuman(out io.Writer, d diagnosisResource) {
	_, _ = fmt.Fprintf(out, "confidence:  %s\n", d.Confidence)
	if d.DeployAttemptID != "" {
		_, _ = fmt.Fprintf(out, "deploy id:   %s\n", d.DeployAttemptID)
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", d.Explanation)
	_, _ = fmt.Fprintf(out, "\nsuggested next step:\n  %s\n", d.Suggestion)
	if len(d.MatchedSignals) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\nmatched signals:")
	for _, s := range d.MatchedSignals {
		_, _ = fmt.Fprintf(out, "  [%s] %s\n", s.Source, s.Excerpt)
	}
}

// printResourceRecommendationHuman prints "apps resource-recommendation"
// and "databases resource-recommendation" output; label distinguishes the
// two ("app" or "database") since the wire type itself is shared.
func printResourceRecommendationHuman(out io.Writer, label string, r resourceRecommendationResource) {
	_, _ = fmt.Fprintf(out, "%s:      %s\n", label, r.ServiceName)
	_, _ = fmt.Fprintf(out, "lookback: %s\n", r.LookbackWindow)
	if r.OOMDetectedAt != "" {
		_, _ = fmt.Fprintf(out, "\nOOM signal: killed on %s\n  %s\n", r.OOMDetectedAt, r.OOMExcerpt)
	}
	_, _ = fmt.Fprintln(out, "\nmemory:")
	printDimensionRecommendationHuman(out, r.Memory, formatRecommendationBytes)
	_, _ = fmt.Fprintln(out, "\ncpu:")
	printDimensionRecommendationHuman(out, r.CPU, formatRecommendationNanoCPUs)
}

func printDimensionRecommendationHuman(out io.Writer, d dimensionRecommendationResource, format func(int64) string) {
	_, _ = fmt.Fprintf(out, "  current limit:    %s\n", format(d.CurrentLimit))
	_, _ = fmt.Fprintf(out, "  samples:          %d (confidence: %s)\n", d.SampleCount, d.Confidence)
	if d.SampleCount > 0 {
		_, _ = fmt.Fprintf(out, "  p95 / p99 usage:  %s / %s\n", format(int64(d.P95Usage)), format(int64(d.P99Usage)))
	}
	if d.Action != "" {
		_, _ = fmt.Fprintf(out, "  suggested action: %s to roughly %s\n", d.Action, format(d.SuggestedLimit))
	}
	_, _ = fmt.Fprintf(out, "  reason:           %s\n", d.Reason)
}

func formatRecommendationBytes(bytes int64) string {
	if bytes <= 0 {
		return "not set"
	}
	return fmt.Sprintf("%d MiB", bytes/1_048_576)
}

func formatRecommendationNanoCPUs(nanoCPUs int64) string {
	if nanoCPUs <= 0 {
		return "not set"
	}
	return fmt.Sprintf("%.2f cores", float64(nanoCPUs)/1e9)
}

// printAppNetworkHuman prints "apps network" output: the live traffic
// path, container's declared port plus whatever host port Docker
// currently has bound.
func printAppNetworkHuman(out io.Writer, n networkResource) {
	_, _ = fmt.Fprintf(out, "container port:  %d\n", n.ContainerPort)
	if n.Running && n.HostPort > 0 {
		_, _ = fmt.Fprintf(out, "host port:       %d\n", n.HostPort)
	} else {
		_, _ = fmt.Fprintln(out, "host port:       (not running)")
	}
	_, _ = fmt.Fprintf(out, "running:         %t\n", n.Running)
}

// printLogEntriesHuman prints "apps logs" output: one line per entry,
// oldest first (the order handleQueryLogs' underlying store returns
// them in), timestamp and stream prefixed so stdout/stderr lines are
// distinguishable without needing --json.
func printLogEntriesHuman(out io.Writer, entries []logEntryResource) {
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "no log entries in range")
		return
	}
	for _, e := range entries {
		_, _ = fmt.Fprintf(out, "%s %s %s\n", e.Timestamp.Format(time.RFC3339), e.Stream, e.Message)
	}
}

// printAppGroupHuman prints "apps group" output: name's sibling services
// under the same app, plus the group's own rollup status.
func printAppGroupHuman(out io.Writer, g appGroupResource) {
	if g.AppID != "" {
		_, _ = fmt.Fprintf(out, "app_id:  %s\n", g.AppID)
	}
	_, _ = fmt.Fprintf(out, "status:  %s\n", g.Status.Label)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tIMAGE\tPORT")
	for _, s := range g.Services {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\n", s.Name, s.Image, s.Port)
	}
	_ = tw.Flush()
}

// printDeploySpecResultHuman prints "apps deploy-spec" output: one line
// per service key's own outcome, error inline when that key's build or
// deploy failed.
func printDeploySpecResultHuman(out io.Writer, r deploySpecResult) {
	_, _ = fmt.Fprintf(out, "app_id: %s\n", r.AppID)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SERVICE\tIMAGE\tERROR")
	for _, s := range r.Services {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ServiceKey, s.Image, s.Error)
	}
	_ = tw.Flush()
	if !r.AllSucceeded {
		_, _ = fmt.Fprintln(out, "one or more services failed, see the ERROR column above")
	}
}

// printDatabaseHuman prints one database resource in a human-readable,
// non-JSON form ("databases get" output).
func printDatabaseHuman(out io.Writer, d databaseResource) {
	_, _ = fmt.Fprintf(out, "name:     %s\n", d.Name)
	_, _ = fmt.Fprintf(out, "engine:   %s\n", d.Engine)
	_, _ = fmt.Fprintf(out, "version:  %s\n", d.Version)
	if d.NodeID != "" {
		_, _ = fmt.Fprintf(out, "node:     %s\n", d.NodeID)
	}
	if d.Resources != nil {
		if d.Resources.MemoryBytes > 0 {
			_, _ = fmt.Fprintf(out, "memory:   %d bytes\n", d.Resources.MemoryBytes)
		}
		if d.Resources.NanoCPUs > 0 {
			_, _ = fmt.Fprintf(out, "cpu:      %.2f cores\n", float64(d.Resources.NanoCPUs)/1e9)
		}
	}
	if d.PubliclyAccessible {
		_, _ = fmt.Fprintf(out, "public:   yes (port %d)\n", d.PublicPort)
	}
	if d.BackupSchedule != "" {
		_, _ = fmt.Fprintf(out, "backup:   %s (target %s)\n", d.BackupSchedule, d.BackupTargetID)
	}
}

// printDatabasesTable prints a compact, aligned table of databases
// ("databases list" output), the same shape printAppsTable uses for apps.
func printDatabasesTable(out io.Writer, dbs []databaseResource) {
	if len(dbs) == 0 {
		_, _ = fmt.Fprintln(out, "no databases")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tENGINE\tVERSION\tNODE")
	for _, d := range dbs {
		node := d.NodeID
		if node == "" {
			node = "(local)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Name, d.Engine, d.Version, node)
	}
	_ = tw.Flush()
}

// printSystemStatusHuman prints "status" output. docker is the headline
// line since an unreachable daemon is the single most fundamental
// prerequisite this platform has: everything else here degrades
// gracefully, Docker being down does not.
func printSystemStatusHuman(out io.Writer, s systemStatusResource) {
	if s.DockerConnected {
		_, _ = fmt.Fprintln(out, "docker:              connected")
	} else if s.DockerError != "" {
		_, _ = fmt.Fprintf(out, "docker:              UNREACHABLE (%s)\n", s.DockerError)
	} else {
		_, _ = fmt.Fprintln(out, "docker:              unknown (no Docker reachability check configured)")
	}
	_, _ = fmt.Fprintf(out, "secrets configured:  %t\n", s.SecretsConfigured)
	_, _ = fmt.Fprintf(out, "telemetry configured: %t\n", s.TelemetryConfigured)
	_, _ = fmt.Fprintf(out, "alerts configured:   %t\n", s.AlertsConfigured)
	if s.DataDirTotalBytes > 0 {
		_, _ = fmt.Fprintf(out, "data dir:            %d free of %d bytes\n", s.DataDirFreeBytes, s.DataDirTotalBytes)
	}
}

// printSystemDoctorHuman prints "doctor" output: one row per check, no
// color, the same plain-text convention printSystemStatusHuman's own
// UNREACHABLE line above already uses instead of ANSI codes.
func printSystemDoctorHuman(out io.Writer, d systemDoctorResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATUS\tCHECK\tMESSAGE")
	for _, c := range d.Checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", doctorStatusLabel(c.Status), c.Name, c.Message)
	}
	_ = tw.Flush()

	if d.OK {
		_, _ = fmt.Fprintln(out, "\nAll checks passed.")
	} else {
		_, _ = fmt.Fprintln(out, "\nOne or more checks failed. See above.")
	}
}

// doctorStatusLabel upper-cases a check's status for the human table,
// the same emphasis-through-caps convention printSystemStatusHuman's
// own "UNREACHABLE" already uses in place of color.
func doctorStatusLabel(status string) string {
	switch status {
	case "ok":
		return "OK"
	case "warn":
		return "WARN"
	case "fail":
		return "FAIL"
	default:
		return "UNKNOWN"
	}
}

// printUpdatesHuman prints "version" output.
func printUpdatesHuman(out io.Writer, u updatesResource) {
	_, _ = fmt.Fprintf(out, "running version:    %s\n", u.CurrentVersion)
	if u.LatestVersion == nil {
		_, _ = fmt.Fprintln(out, "latest release:      none published yet")
		return
	}
	_, _ = fmt.Fprintf(out, "latest release:      %s\n", *u.LatestVersion)
	if u.UpdateAvailable {
		_, _ = fmt.Fprintln(out, "update available:   yes")
		if u.ReleaseURL != nil {
			_, _ = fmt.Fprintf(out, "release url:          %s\n", *u.ReleaseURL)
		}
	} else {
		_, _ = fmt.Fprintln(out, "update available:   no, up to date")
	}
}

// printNodeHuman prints one node resource in a human-readable, non-JSON
// form ("nodes get" output).
func printNodeHuman(out io.Writer, n nodeResource) {
	_, _ = fmt.Fprintf(out, "id:                      %s\n", n.ID)
	_, _ = fmt.Fprintf(out, "name:                    %s\n", n.Name)
	if n.Address != "" {
		_, _ = fmt.Fprintf(out, "address:                 %s\n", n.Address)
	}
	_, _ = fmt.Fprintf(out, "status:                  %s\n", n.Status)
	if n.CertFingerprint != "" {
		_, _ = fmt.Fprintf(out, "cert fingerprint:        %s\n", n.CertFingerprint)
	}
	if n.JoinedAt != nil {
		_, _ = fmt.Fprintf(out, "joined at:               %s\n", n.JoinedAt.Format(time.RFC3339))
	}
	if n.LastSeenAt != nil {
		_, _ = fmt.Fprintf(out, "last seen at:            %s\n", n.LastSeenAt.Format(time.RFC3339))
	}
	_, _ = fmt.Fprintf(out, "schedulable:             %t\n", n.Schedulable)
	_, _ = fmt.Fprintf(out, "accepts app workloads:   %t\n", n.AcceptsAppWorkloads)
	_, _ = fmt.Fprintf(out, "accepts build workloads: %t\n", n.AcceptsBuildWorkloads)
	_, _ = fmt.Fprintf(out, "created at:              %s\n", n.CreatedAt.Format(time.RFC3339))
	if n.AlertStatus != nil {
		_, _ = fmt.Fprintf(out, "alert status:\n")
		_, _ = fmt.Fprintf(out, "  patch status:          %s\n", n.AlertStatus.PatchStatus)
		_, _ = fmt.Fprintf(out, "  node disk space:       %s\n", n.AlertStatus.NodeDiskSpace)
		_, _ = fmt.Fprintf(out, "  node resource usage:   %s\n", n.AlertStatus.NodeResourceUsage)
	}
}

// printNodesTable prints a compact, aligned table of nodes ("nodes list"
// output), the same shape printAppsTable/printDatabasesTable use.
func printNodesTable(out io.Writer, nodes []nodeResource) {
	if len(nodes) == 0 {
		_, _ = fmt.Fprintln(out, "no nodes")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tADDRESS\tSTATUS\tSCHEDULABLE\tCREATED")
	for _, n := range nodes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\n", n.ID, n.Name, n.Address, n.Status, n.Schedulable, n.CreatedAt.Format(time.RFC3339))
	}
	_ = tw.Flush()
}
