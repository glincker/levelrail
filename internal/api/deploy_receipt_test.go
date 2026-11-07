package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deployreceipt"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeployReceipt_NamesRequesterAndApprover(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	started := time.Now().UTC().Add(-10 * time.Minute)
	h.seedDeploy(t, "dep_r", started)
	ap := store.DeployApproval{
		ID: "apr_r", ServiceName: "web", Action: store.DeployApprovalActionDeploy, Image: "img:dep_r", Status: store.DeployApprovalStatusApproved,
		RequestedByType: "user", RequestedBy: "u1", RequestedByName: "Ana",
		ApprovedByType: "user", ApprovedBy: "u2", ApprovedByName: "Bo",
		CreatedAt: started.Add(-time.Minute).Format(time.RFC3339Nano), ExpiresAt: started.Add(time.Hour).Format(time.RFC3339Nano),
		DecidedAt: started.Add(-30 * time.Second).Format(time.RFC3339Nano),
	}
	if err := h.db.SaveDeployApproval(context.Background(), ap); err != nil {
		t.Fatal(err)
	}

	rec := h.get(t, "/api/v1/apps/web/deployments/dep_r/receipt")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got deployreceipt.Receipt
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || got.Signed || got.Commit.SHA != "abcdef1234567" {
		t.Fatalf("receipt = %+v", got)
	}
	if got.Approval == nil || got.Approval.Approver == nil || got.Approval.Approver.Name != "Bo" || got.Approval.Requester.Name != "Ana" || got.Approval.MatchedBy != deployreceipt.MatchedImageAndTime {
		t.Fatalf("people not named: %+v", got)
	}
	if got.SBOM.Recorded || got.SBOM.Note != deployreceipt.NotRecorded {
		t.Fatalf("sbom = %+v", got.SBOM)
	}

	md := h.get(t, "/api/v1/apps/web/deployments/dep_r/receipt?format=md")
	if md.Code != http.StatusOK || !strings.Contains(md.Body.String(), "Approved by: Bo") {
		t.Fatalf("markdown = %d %s", md.Code, md.Body.String())
	}
	if rec := h.get(t, "/api/v1/apps/other/deployments/dep_r/receipt"); rec.Code != http.StatusNotFound {
		t.Fatalf("wrong app status = %d", rec.Code)
	}
	if rec := h.get(t, "/api/v1/apps/web/deployments/dep_r/receipt?format=xml"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad format status = %d", rec.Code)
	}
}
