package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// appDiagnosticViews are the read-only per-app routes GetAppDiagnostic may call;
// the allowlist keeps a typo or a crafted view name from reaching another route.
var appDiagnosticViews = map[string]bool{
	"effective-config": true,
	"pending-changes":  true,
	"failure-context":  true,
	"rollback-targets": true,
	"image-status":     true,
}

// GetAppDiagnostic calls GET /api/v1/apps/{name}/{view} and returns the body as
// the server sent it, so --json output is the API payload with no re-encoding.
func (c *Client) GetAppDiagnostic(ctx context.Context, name, view string, q url.Values) (json.RawMessage, error) {
	if !appDiagnosticViews[view] {
		return nil, fmt.Errorf("unknown app diagnostic view %q", view)
	}
	path := "/api/v1/apps/" + PathEscape(name) + "/" + view
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
