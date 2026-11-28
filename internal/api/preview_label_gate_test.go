package api

import (
	"testing"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/webhook"
)

func TestPreviewLabelGateMessage(t *testing.T) {
	base := webhook.PullRequestEvent{Number: 7, LabelsKnown: true, Labels: []string{"Preview"}}
	if m := previewLabelGateMessage("", base); m != "" {
		t.Fatalf("no gate must pass, got %q", m)
	}
	if m := previewLabelGateMessage("preview", base); m != "" {
		t.Fatalf("label present must pass, got %q", m)
	}
	none := webhook.PullRequestEvent{Number: 7, LabelsKnown: true}
	if m := previewLabelGateMessage("preview", none); m == "" {
		t.Fatal("missing label must be gated")
	}
	other := webhook.PullRequestEvent{Number: 7, LabelsKnown: true, Labels: []string{"preview", "bug"}, LabelAdded: "bug"}
	if m := previewLabelGateMessage("preview", other); m == "" {
		t.Fatal("another label being added must not redeploy")
	}
	added := webhook.PullRequestEvent{Number: 7, LabelsKnown: true, Labels: []string{"preview"}, LabelAdded: "preview"}
	if m := previewLabelGateMessage("preview", added); m != "" {
		t.Fatalf("adding the gate label must pass, got %q", m)
	}
	if m := previewLabelGateMessage("", added); m == "" {
		t.Fatal("a labeled delivery with no gate must be ignored")
	}
	if m := previewLabelGateMessage("preview", webhook.PullRequestEvent{Number: 7}); m != "" {
		t.Fatalf("providers without labels are never gated, got %q", m)
	}
}

func TestApplyPreviewLabelField(t *testing.T) {
	var s store.PreviewAppSettings
	l := "  preview "
	if msg := applyPreviewLabelField(&s, setPreviewPolicyRequest{RequireLabel: &l}); msg != "" || s.RequireLabel != "preview" {
		t.Fatalf("msg=%q val=%q", msg, s.RequireLabel)
	}
	bad := "a\nb"
	if msg := applyPreviewLabelField(&s, setPreviewPolicyRequest{RequireLabel: &bad}); msg == "" {
		t.Fatal("control characters must be rejected")
	}
	empty := ""
	if msg := applyPreviewLabelField(&s, setPreviewPolicyRequest{RequireLabel: &empty}); msg != "" || s.RequireLabel != "" {
		t.Fatalf("empty must clear, msg=%q", msg)
	}
}
