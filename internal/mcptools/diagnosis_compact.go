package mcptools

import (
	"context"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

// compactDiagnosis is the short why-it-failed answer attached to a failed
// deploy result, so an agent does not need a second tool call.
type compactDiagnosis struct {
	Class      string `json:"class,omitempty"`
	Cause      string `json:"cause,omitempty"`
	Fix        string `json:"fix,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
}

func diagField(s string, limit int) string {
	return untrusted.Truncate(untrusted.Redact(untrusted.Clean(strings.TrimSpace(s))), limit)
}

// buildDiagnosis merges the deploy's classified failure with the app
// diagnosis. The diagnosis call is best effort: on error the failure alone is used.
func buildDiagnosis(ctx context.Context, client *apiclient.Client, app, deployID string, failure *apiclient.DeployFailure) *compactDiagnosis {
	limits := untrusted.LimitsFromEnv()
	d := &compactDiagnosis{}
	if failure != nil {
		d.Class = diagField(failure.Code, limits.Field)
		d.Cause = diagField(failure.Cause, limits.Field)
		d.Fix = diagField(failure.SuggestedFix, limits.Field)
		d.Excerpt = diagField(failure.LogExcerpt, limits.Field)
		d.Retryable = failure.Retryable
	}
	res, err := client.DiagnoseApp(ctx, app, deployID)
	if err == nil {
		d.Confidence = diagField(res.Confidence, limits.Field)
		if len(res.Causes) > 0 {
			c := res.Causes[0]
			if d.Class == "" {
				d.Class = diagField(c.Code, limits.Field)
			}
			if d.Cause == "" {
				d.Cause = diagField(c.Explanation, limits.Field)
			}
			if d.Fix == "" && len(c.Fixes) > 0 {
				d.Fix = diagField(c.Fixes[0].Label, limits.Field)
			}
		} else if d.Cause == "" {
			d.Cause = diagField(res.Explanation, limits.Field)
			d.Fix = diagField(res.Suggestion, limits.Field)
		}
	}
	if *d == (compactDiagnosis{}) {
		return nil
	}
	return d
}
