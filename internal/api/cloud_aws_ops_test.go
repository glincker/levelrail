package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

const awsOpsBase = awsSvcBase

func TestCloudAWSOps_FlagOffAnswers409(t *testing.T) {
	rt, db := awsServicesRouter(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: "<R/>"} })
	root := loginTestSession(t, rt, db)
	for _, p := range []string{"ebs/volumes", "ebs/snapshots", "cloudwatch/metrics?instance_id=i-1", "cloudwatch/log-groups", "cloudwatch/log-events?group=/a", "idle"} {
		if code, _ := teamDo(t, rt, http.MethodGet, awsOpsBase+p, "", withCookie(root)); code != http.StatusConflict {
			t.Errorf("GET %s with flag off = %d", p, code)
		}
	}
	if code, _ := teamDo(t, rt, http.MethodPost, awsOpsBase+"ebs/snapshots", `{"volume_id":"vol-1"}`, withCookie(root)); code != http.StatusConflict {
		t.Errorf("create with flag off = %d", code)
	}
	if code, _ := teamDo(t, rt, http.MethodDelete, awsOpsBase+"ebs/snapshots/snap-1", "", withCookie(root)); code != http.StatusConflict {
		t.Errorf("delete with flag off = %d", code)
	}
}

func TestCloudAWSOps_DeleteRefusesUnmanaged(t *testing.T) {
	var actions []string
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		actions = append(actions, rdsAction(c))
		return awsxtest.Reply{Body: `<R><snapshotSet><item><snapshotId>snap-1</snapshotId><volumeId>vol-1</volumeId></item></snapshotSet></R>`}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardEBS)
	root := loginTestSession(t, rt, db)
	if code, out := teamDo(t, rt, http.MethodDelete, awsOpsBase+"ebs/snapshots/snap-1", "", withCookie(root)); code != http.StatusConflict {
		t.Fatalf("unmanaged delete = %d %s", code, out)
	}
	for _, a := range actions {
		if a == "DeleteSnapshot" {
			t.Fatal("unmanaged snapshot was deleted")
		}
	}
	if code, _ := teamDo(t, rt, http.MethodPost, awsOpsBase+"ebs/snapshots", `{"volume_id":"vol-1","instance_id":"i-1"}`, withCookie(root)); code != http.StatusBadRequest {
		t.Fatalf("both sources = %d", code)
	}
}

func TestCloudAWSOps_MetricsValidatesAndCaches(t *testing.T) {
	var calls int
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		if strings.Contains(c.Body, "GetMetricData") {
			calls++
		}
		return awsxtest.Reply{Body: `<R><GetMetricDataResult><MetricDataResults><member><Id>m0</Id><Timestamps><member>2026-11-21T11:05:00Z</member></Timestamps><Values><member>3</member></Values></member></MetricDataResults></GetMetricDataResult></R>`}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardCloudWatch)
	root := loginTestSession(t, rt, db)
	if code, _ := teamDo(t, rt, http.MethodGet, awsOpsBase+"cloudwatch/metrics?instance_id=i-1&names=bad%20name", "", withCookie(root)); code != http.StatusBadRequest {
		t.Fatalf("bad name = %d", code)
	}
	for i := 0; i < 2; i++ {
		if code, out := teamDo(t, rt, http.MethodGet, awsOpsBase+"cloudwatch/metrics?instance_id=i-cache-test-1", "", withCookie(root)); code != http.StatusOK || !strings.Contains(out, `"v":3`) {
			t.Fatalf("metrics = %d %s", code, out)
		}
	}
	if calls != 1 {
		t.Fatalf("billed reads = %d, want 1 (second view cached)", calls)
	}
}
