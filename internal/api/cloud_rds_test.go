package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

const rdsBase = awsSvcBase + "rds/"

func rdsAction(c awsxtest.Call) string {
	v, _ := url.ParseQuery(c.Body)
	return v.Get("Action")
}

func TestCloudRDS_FlagOffAnswers409(t *testing.T) {
	rt, db := awsServicesRouter(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: "<R/>"} })
	root := loginTestSession(t, rt, db)
	for _, p := range []string{"instances", "clusters", "snapshots"} {
		if code, _ := teamDo(t, rt, http.MethodGet, rdsBase+p, "", withCookie(root)); code != http.StatusConflict {
			t.Errorf("GET %s with flag off = %d", p, code)
		}
	}
	if code, _ := teamDo(t, rt, http.MethodPost, rdsBase+"instances", `{"id":"a","engine":"postgres"}`, withCookie(root)); code != http.StatusConflict {
		t.Errorf("create with flag off = %d", code)
	}
}

func TestCloudRDS_CreateRefusedOverCap(t *testing.T) {
	var writes int
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		if rdsAction(c) == "CreateDBInstance" {
			writes++
		}
		return awsxtest.Reply{Body: "<R/>"}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardRDS)
	for k, v := range map[string]string{runtimesettings.KeyCloudGuardPerCreateUSD: "50", runtimesettings.KeyCloudGuardExpiresHours: "24", runtimesettings.KeyCloudGuardInstanceTypes: `["db.t4g.micro"]`} {
		if _, err := rt.runtimeSettings.Set(context.Background(), k, json.RawMessage(v), "test"); err != nil {
			t.Fatal(err)
		}
	}
	root := loginTestSession(t, rt, db)
	body := `{"id":"db1","engine":"postgres","class":"db.t4g.micro","master_username":"app","allocated_gb":20,"estimated_monthly_usd":500}`
	if code, out := teamDo(t, rt, http.MethodPost, rdsBase+"instances", body, withCookie(root)); code != http.StatusConflict {
		t.Fatalf("over cap create = %d %s", code, out)
	}
	if writes != 0 {
		t.Fatal("a refused create reached AWS")
	}
	multi := `{"id":"db1","engine":"postgres","class":"db.t4g.micro","master_username":"app","allocated_gb":20,"multi_az":true,"estimated_monthly_usd":10}`
	if code, _ := teamDo(t, rt, http.MethodPost, rdsBase+"instances", multi, withCookie(root)); code != http.StatusConflict {
		t.Fatalf("multi az without confirm = %d", code)
	}
	ok := strings.Replace(body, `"estimated_monthly_usd":500`, `"estimated_monthly_usd":10`, 1)
	if code, out := teamDo(t, rt, http.MethodPost, rdsBase+"instances", ok, withCookie(root)); code != http.StatusAccepted || writes != 1 {
		t.Fatalf("allowed create = %d %s writes=%d", code, out, writes)
	}
}

func TestCloudRDS_AdoptWritesTagOnly(t *testing.T) {
	var actions []string
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		actions = append(actions, rdsAction(c))
		if rdsAction(c) == "DescribeDBInstances" {
			return awsxtest.Reply{Body: `<R><DescribeDBInstancesResult><DBInstances><DBInstance><DBInstanceIdentifier>x</DBInstanceIdentifier><DBInstanceArn>arn:x</DBInstanceArn></DBInstance></DBInstances></DescribeDBInstancesResult></R>`}
		}
		return awsxtest.Reply{Body: "<R/>"}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardRDS)
	root := loginTestSession(t, rt, db)
	if code, out := teamDo(t, rt, http.MethodPost, rdsBase+"instances/x/actions", `{"action":"adopt"}`, withCookie(root)); code != http.StatusAccepted {
		t.Fatalf("adopt = %d %s", code, out)
	}
	if len(actions) != 2 || actions[1] != "AddTagsToResource" {
		t.Fatalf("adopt calls = %v", actions)
	}
	actions = nil
	if code, _ := teamDo(t, rt, http.MethodPost, rdsBase+"instances/x/actions", `{"action":"stop"}`, withCookie(root)); code != http.StatusConflict {
		t.Fatalf("stop on unmanaged = %d", code)
	}
	for _, a := range actions {
		if a == "StopDBInstance" {
			t.Fatal("unmanaged instance was stopped")
		}
	}
}

func TestCloudRDS_ClusterStopSurfacesAutoRestart(t *testing.T) {
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		if rdsAction(c) == "DescribeDBClusters" {
			return awsxtest.Reply{Body: `<R><DescribeDBClustersResult><DBClusters><DBCluster><DBClusterIdentifier>k</DBClusterIdentifier><DBClusterArn>arn:k</DBClusterArn><TagList><Tag><Key>levelrail:managed</Key><Value>true</Value></Tag></TagList></DBCluster></DBClusters></DescribeDBClustersResult></R>`}
		}
		return awsxtest.Reply{Body: "<R/>"}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardRDS)
	root := loginTestSession(t, rt, db)
	code, out := teamDo(t, rt, http.MethodPost, rdsBase+"clusters/k/actions", `{"action":"stop"}`, withCookie(root))
	if code != http.StatusAccepted || !strings.Contains(out, `"auto_restart_days":7`) {
		t.Fatalf("cluster stop = %d %s", code, out)
	}
}
