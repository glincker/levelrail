package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deployreceipt"
	"github.com/GLINCKER/levelrail/internal/store"
)

type failingApprovals struct{ DeployApprovalStore }

func (failingApprovals) ListDeployApprovals(context.Context, string, string) ([]store.DeployApproval, error) {
	return nil, errors.New("boom")
}

type failingGate struct{ DeployGateStore }

func (failingGate) ListGateEvents(context.Context, string) ([]store.DeployGateEvent, error) {
	return nil, errors.New("boom")
}

func TestDeployReceipt_StaleApprovalIsNotNamed(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	started := time.Now().UTC().Add(-10 * time.Minute)
	h.seedDeploy(t, "dep_s", started, store.DeployAttemptStatusSucceeded)
	old := started.AddDate(0, -7, 0)
	if err := h.db.SaveDeployApproval(context.Background(), store.DeployApproval{
		ID: "apr_old", ServiceName: "web", Action: store.DeployApprovalActionDeploy, Image: "img:dep_s", Status: store.DeployApprovalStatusApproved,
		RequestedByName: "Ana", ApprovedByName: "Bo", ApprovedBy: "u2",
		CreatedAt: old.Format(time.RFC3339Nano), ExpiresAt: old.Add(time.Hour).Format(time.RFC3339Nano), DecidedAt: old.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	var got deployreceipt.Receipt
	if err := json.Unmarshal(h.get(t, "/api/v1/apps/web/deployments/dep_s/receipt").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Approval != nil || got.Requester != nil {
		t.Fatalf("stale approval leaked: %+v", got)
	}
}

func TestDeployReceipt_FailedLookupsAreIncomplete(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	h.seedDeploy(t, "dep_f", time.Now().UTC().Add(-time.Minute), store.DeployAttemptStatusSucceeded)
	h.rt.deployApprovals = failingApprovals{h.rt.deployApprovals}
	h.rt.deployGate = failingGate{h.rt.deployGate}
	var got deployreceipt.Receipt
	if err := json.Unmarshal(h.get(t, "/api/v1/apps/web/deployments/dep_f/receipt").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Incomplete || len(got.LookupErrors) != 2 {
		t.Fatalf("receipt = %+v", got)
	}
	md := h.get(t, "/api/v1/apps/web/deployments/dep_f/receipt?format=md").Body.String()
	if !strings.Contains(md, "approval lookup failed, unknown") || strings.Contains(md, "no approval on record") {
		t.Fatalf("markdown = %s", md)
	}
}
