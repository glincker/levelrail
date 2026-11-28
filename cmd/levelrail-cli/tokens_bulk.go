package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// bulkRevokeFilter mirrors the server's token filter used by bulk revoke.
type bulkRevokeFilter struct {
	Q             string `json:"q,omitempty"`
	Status        string `json:"status,omitempty"`
	Ability       string `json:"ability,omitempty"`
	Name          string `json:"name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	ExpiresBefore string `json:"expires_before,omitempty"`
}

func (f bulkRevokeFilter) empty() bool { return f == bulkRevokeFilter{} }

type bulkRevokeRequest struct {
	IDs          []string          `json:"ids,omitempty"`
	Filter       *bulkRevokeFilter `json:"filter,omitempty"`
	ConfirmCount *int              `json:"confirm_count,omitempty"`
	DryRun       bool              `json:"dry_run"`
}

type bulkRevokeResponse struct {
	DryRun  bool     `json:"dry_run"`
	Count   int      `json:"count"`
	Sample  []string `json:"sample"`
	Revoked int      `json:"revoked"`
	Max     int      `json:"max"`
}

// bulkRevokeAPITokenMessage is shown when the CLI would call the session only route with an API token.
const bulkRevokeAPITokenMessage = "bulk revoke needs a signed-in session, but this command is using an API token (--token, the environment or the saved profile). Sign in by passing --username and --password (leave --password out to be asked) and run it again"

// parseTokenFilter reads --filter 'k=v,k=v' with the keys the server accepts.
func parseTokenFilter(raw string) (bulkRevokeFilter, error) {
	var f bulkRevokeFilter
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok || v == "" {
			return f, fmt.Errorf("--filter entry %q must look like key=value", part)
		}
		switch strings.TrimSpace(k) {
		case "q":
			f.Q = v
		case "status":
			f.Status = v
		case "ability":
			f.Ability = v
		case "name":
			f.Name = v
		case "kind":
			f.Kind = v
		case "expires_before":
			f.ExpiresBefore = v
		default:
			return f, fmt.Errorf("--filter key %q is not one of q, status, ability, name, kind, expires_before", k)
		}
	}
	return f, nil
}

type bulkRevokeOpts struct {
	ids    []string
	filter *bulkRevokeFilter
	dryRun bool
	yes    bool
}

// runBulkRevoke always asks the server for a dry run first, then revokes with the count the user saw.
func runBulkRevoke(ctx context.Context, sc *authSessionClient, o bulkRevokeOpts, stdin io.Reader, stderr io.Writer) (bulkRevokeResponse, error) {
	preview, err := sc.BulkRevokeTokens(ctx, bulkRevokeRequest{IDs: o.ids, Filter: o.filter, DryRun: true})
	if err != nil {
		return preview, fmt.Errorf("preview bulk revoke: %w", err)
	}
	if preview.Count == 0 || o.dryRun {
		return preview, nil
	}
	_, _ = fmt.Fprintf(stderr, "%d tokens will be revoked: %s%s\n", preview.Count, strings.Join(preview.Sample, ", "), moreLabel(preview.Count, len(preview.Sample)))
	if err := confirmAction(stdin, stderr, fmt.Sprintf("Revoke %d tokens", preview.Count), o.yes, "--yes"); err != nil {
		return preview, err
	}
	count := preview.Count
	res, err := sc.BulkRevokeTokens(ctx, bulkRevokeRequest{IDs: o.ids, Filter: o.filter, ConfirmCount: &count})
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return res, newValidationError("the number of matching tokens changed since the preview, nothing was revoked: run the command again")
	}
	if err != nil {
		return res, fmt.Errorf("bulk revoke: %w", err)
	}
	return res, nil
}

func moreLabel(count, shown int) string {
	if count <= shown {
		return ""
	}
	return fmt.Sprintf(" and %d more", count-shown)
}
