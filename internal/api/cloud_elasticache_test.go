package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const ecBase = awsSvcBase + "elasticache/"

func TestCloudElastiCache_FlagOffAnswers409(t *testing.T) {
	rt, db := awsServicesRouter(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: "<R/>"} })
	root := loginTestSession(t, rt, db)
	for _, p := range []string{"replication-groups", "clusters", "snapshots"} {
		if code, _ := teamDo(t, rt, http.MethodGet, ecBase+p, "", withCookie(root)); code != http.StatusConflict {
			t.Errorf("GET %s with flag off = %d", p, code)
		}
	}
}

func TestCloudElastiCache_DeleteNeedsConfirmAndManaged(t *testing.T) {
	var deletes int
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		switch rdsAction(c) {
		case "DescribeReplicationGroups":
			return awsxtest.Reply{Body: `<R><DescribeReplicationGroupsResult><ReplicationGroups><ReplicationGroup><ReplicationGroupId>rg1</ReplicationGroupId><ARN>arn:rg1</ARN></ReplicationGroup></ReplicationGroups></DescribeReplicationGroupsResult></R>`}
		case "DeleteReplicationGroup":
			deletes++
		}
		return awsxtest.Reply{Body: "<R/>"}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardElastiCache)
	root := loginTestSession(t, rt, db)
	if code, _ := teamDo(t, rt, http.MethodDelete, ecBase+"replication-groups/rg1?confirm=nope", "", withCookie(root)); code != http.StatusBadRequest {
		t.Fatalf("wrong confirm = %d", code)
	}
	if code, out := teamDo(t, rt, http.MethodDelete, ecBase+"replication-groups/rg1?confirm=rg1", "", withCookie(root)); code != http.StatusConflict || !strings.Contains(out, "not managed") {
		t.Fatalf("unmanaged delete = %d %s", code, out)
	}
	if deletes != 0 {
		t.Fatal("a refused delete reached AWS")
	}
}
