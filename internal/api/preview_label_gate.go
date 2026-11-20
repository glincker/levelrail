package api

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

const maxPreviewLabelLen = 100

// previewLabelGateMessage returns why a pull request event is skipped by the
// label gate, or "" to go ahead. Providers whose payload has no labels are never gated.
func previewLabelGateMessage(label string, ev webhook.PullRequestEvent) string {
	if label == "" {
		if ev.LabelAdded != "" {
			return fmt.Sprintf("ignored: pull request #%d was labeled and no label gate is set\n", ev.Number)
		}
		return ""
	}
	if !ev.LabelsKnown {
		return ""
	}
	// A "labeled" delivery only counts when it added the gate label, so other labels do not redeploy.
	if ev.LabelAdded != "" && !strings.EqualFold(ev.LabelAdded, label) {
		return fmt.Sprintf("ignored: label %q is not the preview label %q\n", ev.LabelAdded, label)
	}
	if !ev.HasLabel(label) {
		return fmt.Sprintf("ignored: pull request #%d has no %q label, previews build once it is added\n", ev.Number, label)
	}
	return ""
}

// applyPreviewLabelField validates the label gate of a policy update.
func applyPreviewLabelField(s *store.PreviewAppSettings, req setPreviewPolicyRequest) string {
	if req.RequireLabel == nil {
		return ""
	}
	label := strings.TrimSpace(*req.RequireLabel)
	if len(label) > maxPreviewLabelLen {
		return fmt.Sprintf("require_label must be at most %d characters", maxPreviewLabelLen)
	}
	if strings.IndexFunc(label, unicode.IsControl) >= 0 {
		return "require_label must not contain control characters"
	}
	s.RequireLabel = label
	return ""
}
