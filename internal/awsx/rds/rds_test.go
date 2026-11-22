package rds

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
)

var testNow = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func guardFor(cfg cloudguard.Config) *cloudguard.Guard {
	cfg.Now = func() time.Time { return testNow }
	return cloudguard.New(func() cloudguard.Config { return cfg }, nil)
}

func okCfg() cloudguard.Config {
	return cloudguard.Config{InstanceTypes: []string{"db.t4g.micro"}, ServicesEnabled: map[string]bool{"rds": true}, DefaultTTL: 24 * time.Hour}
}

func form(c awsxtest.Call) url.Values { v, _ := url.ParseQuery(c.Body); return v }

func TestDescribePaginatesAndParses(t *testing.T) {
	body := func(marker, id string) string {
		return `<R><DescribeDBInstancesResult>` + marker + `<DBInstances><DBInstance><DBInstanceIdentifier>` + id + `</DBInstanceIdentifier><Endpoint><Address>h</Address><Port>5432</Port></Endpoint><TagList><Tag><Key>levelrail:managed</Key><Value>true</Value></Tag></TagList></DBInstance></DBInstances></DescribeDBInstancesResult></R>`
	}
	c2, rec2 := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			return awsxtest.Reply{Body: body("<Marker>m1</Marker>", "a")}
		}
		return awsxtest.Reply{Body: body("", "b")}
	})
	list, err := New(c2, guardFor(okCfg())).DescribeDBInstances(context.Background(), "us-east-1")
	if err != nil || len(list) != 2 || list[1].ID != "b" || !list[0].Managed() || list[0].Port != 5432 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if form(rec2.Calls()[1])["Marker"][0] != "m1" {
		t.Fatal("second page must carry the marker")
	}
}

func TestUnmanagedIsReadOnly(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: strings.NewReplacer("%s", "").Replace(`<R><DescribeDBInstancesResult><DBInstances><DBInstance><DBInstanceIdentifier>x</DBInstanceIdentifier></DBInstance></DBInstances></DescribeDBInstancesResult></R>`)}
	})
	cl := New(c, guardFor(okCfg()))
	if err := cl.Stop(context.Background(), "us-east-1", "x", ""); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("stop err=%v", err)
	}
	if err := cl.CreateSnapshot(context.Background(), "us-east-1", "x", "s", Meta{}); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("snapshot err=%v", err)
	}
	for _, call := range rec.Calls() {
		if a := form(call).Get("Action"); a != "DescribeDBInstances" {
			t.Fatalf("unexpected write %s", a)
		}
	}
}

func TestAdoptWritesTagsOnly(t *testing.T) {
	c, rec := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			return awsxtest.Reply{Body: `<R><DescribeDBInstancesResult><DBInstances><DBInstance><DBInstanceIdentifier>x</DBInstanceIdentifier><DBInstanceArn>arn:x</DBInstanceArn></DBInstance></DBInstances></DescribeDBInstancesResult></R>`}
		}
		return awsxtest.Reply{Body: `<AddTagsToResourceResponse/>`}
	})
	if err := New(c, guardFor(okCfg())).Adopt(context.Background(), "us-east-1", "x", Meta{Owner: "o", Env: "e", Project: "p", ID: "x"}); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls()
	if len(calls) != 2 || form(calls[1]).Get("Action") != "AddTagsToResource" {
		t.Fatalf("calls=%d", len(calls))
	}
	if strings.Contains(calls[1].Body, "expires") {
		t.Fatal("adopt must not set a teardown timer")
	}
}

func TestCreateGate(t *testing.T) {
	meta := Meta{Owner: "o", Env: "e", Project: "p", ID: "db1"}
	base := CreateInput{Account: "a", Region: "us-east-1", ID: "db1", Engine: "postgres", Class: "db.t4g.micro", MasterUsername: "app", AllocatedGB: 20, Meta: meta}
	cases := []struct {
		name string
		mod  func(*CreateInput)
		cfg  func(*cloudguard.Config)
		want error
	}{
		{"ok", nil, nil, nil},
		{"multi az needs confirm", func(i *CreateInput) { i.MultiAZ = true }, nil, ErrNeedsConfirm},
		{"aurora needs confirm", func(i *CreateInput) { i.Engine = "aurora-postgresql" }, nil, ErrNeedsConfirm},
		{"class not allowed", func(i *CreateInput) { i.Class = "db.r6g.16xlarge" }, nil, ErrRefused},
		{"service off", nil, func(c *cloudguard.Config) { c.ServicesEnabled = nil }, ErrRefused},
		{"no timer", nil, func(c *cloudguard.Config) { c.DefaultTTL = 0 }, ErrNoTimer},
		{"over per create cap", func(i *CreateInput) { i.EstimatedMonthlyUSD = 500 }, func(c *cloudguard.Config) { c.PerCreateMaxUSD = 50 }, ErrRefused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := okCfg()
			if tc.cfg != nil {
				tc.cfg(&cfg)
			}
			in := base
			if tc.mod != nil {
				tc.mod(&in)
			}
			c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: `<CreateDBInstanceResponse/>`} })
			err := New(c, guardFor(cfg)).CreateInstance(context.Background(), in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
			if tc.want != nil && len(rec.Calls()) != 0 {
				t.Fatal("a refused create must not reach AWS")
			}
			if tc.want == nil {
				v := form(rec.Calls()[0])
				if v.Get("ManageMasterUserPassword") != "true" || v.Get("PubliclyAccessible") != "false" || v.Get("Tags.Tag.1.Key") == "" {
					t.Fatalf("create params: %v", v)
				}
				if v.Get("MasterUserPassword") != "" {
					t.Fatal("no password may be sent")
				}
			}
		})
	}
}

func TestDeleteNeedsRootAndSnapshot(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: `<R/>`} })
	cl := New(c, guardFor(okCfg()))
	if err := cl.DeleteInstance(context.Background(), DeleteInput{Region: "us-east-1", ID: "x", FinalSnapshotID: "f"}); !errors.Is(err, ErrNeedsRoot) {
		t.Fatal(err)
	}
	if err := cl.DeleteInstance(context.Background(), DeleteInput{Region: "us-east-1", ID: "x", Root: true}); !errors.Is(err, ErrNeedsRoot) {
		t.Fatal(err)
	}
	if len(rec.Calls()) != 0 {
		t.Fatal("no AWS call expected")
	}
}

func TestPlanRestop(t *testing.T) {
	old := testNow.Add(-8 * 24 * time.Hour).Format(time.RFC3339)
	body := `<R><DescribeDBInstancesResult><DBInstances>` +
		`<DBInstance><DBInstanceIdentifier>a</DBInstanceIdentifier><DBInstanceStatus>available</DBInstanceStatus><TagList><Tag><Key>levelrail:managed</Key><Value>true</Value></Tag><Tag><Key>levelrail:stopped_at</Key><Value>` + old + `</Value></Tag></TagList></DBInstance>` +
		`<DBInstance><DBInstanceIdentifier>b</DBInstanceIdentifier><DBInstanceStatus>stopped</DBInstanceStatus><TagList><Tag><Key>levelrail:managed</Key><Value>true</Value></Tag><Tag><Key>levelrail:stopped_at</Key><Value>` + old + `</Value></Tag></TagList></DBInstance>` +
		`<DBInstance><DBInstanceIdentifier>c</DBInstanceIdentifier><DBInstanceStatus>available</DBInstanceStatus></DBInstance>` +
		`</DBInstances></DescribeDBInstancesResult></R>`
	c, _ := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: body} })
	plan, err := New(c, guardFor(okCfg())).WithClock(func() time.Time { return testNow }).PlanRestop(context.Background(), "us-east-1", 7*24*time.Hour)
	if err != nil || len(plan) != 1 || plan[0].ID != "a" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
}

func TestClusterStopRefusesUnmanagedAndTagsManaged(t *testing.T) {
	managed := false
	c, rec := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			tag := ""
			if managed {
				tag = `<TagList><Tag><Key>levelrail:managed</Key><Value>true</Value></Tag></TagList>`
			}
			return awsxtest.Reply{Body: `<R><DescribeDBClustersResult><DBClusters><DBCluster><DBClusterIdentifier>k</DBClusterIdentifier><DBClusterArn>arn:k</DBClusterArn>` + tag + `</DBCluster></DBClusters></DescribeDBClustersResult></R>`}
		}
		return awsxtest.Reply{Body: "<R/>"}
	})
	cl := New(c, guardFor(okCfg()))
	if err := cl.StopCluster(context.Background(), "us-east-1", "k"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("unmanaged stop err=%v", err)
	}
	if len(rec.Calls()) != 1 {
		t.Fatal("unmanaged cluster reached a write")
	}
}

func TestClusterSnapshotsParseAndDeleteNeedsManaged(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `<R><DescribeDBClusterSnapshotsResult><DBClusterSnapshots><DBClusterSnapshot><DBClusterSnapshotIdentifier>s1</DBClusterSnapshotIdentifier><DBClusterIdentifier>k</DBClusterIdentifier><Status>available</Status></DBClusterSnapshot></DBClusterSnapshots></DescribeDBClusterSnapshotsResult></R>`}
	})
	cl := New(c, guardFor(okCfg()))
	list, err := cl.DescribeDBClusterSnapshots(context.Background(), "us-east-1", "k", "")
	if err != nil || len(list) != 1 || list[0].ClusterID != "k" || list[0].Managed() {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if err := cl.DeleteClusterSnapshot(context.Background(), "us-east-1", "s1"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("delete err=%v", err)
	}
	for _, call := range rec.Calls() {
		if form(call).Get("Action") == "DeleteDBClusterSnapshot" {
			t.Fatal("unmanaged snapshot was deleted")
		}
	}
}

func TestPlanClusterRestartWindow(t *testing.T) {
	mgd := `<TagList><Tag><Key>levelrail:managed</Key><Value>true</Value></Tag></TagList>`
	k := func(id, status, at, tags string) string {
		return `<DBCluster><DBClusterIdentifier>` + id + `</DBClusterIdentifier><Status>` + status + `</Status><AutomaticRestartTime>` + at + `</AutomaticRestartTime>` + tags + `</DBCluster>`
	}
	soon := testNow.Add(10 * time.Hour).Format(time.RFC3339)
	far := testNow.Add(100 * time.Hour).Format(time.RFC3339)
	body := `<R><DescribeDBClustersResult><DBClusters>` + k("soon", "stopped", soon, mgd) + k("far", "stopped", far, mgd) +
		k("running", "available", soon, mgd) + k("foreign", "stopped", soon, "") + `</DBClusters></DescribeDBClustersResult></R>`
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: body} })
	plan, err := New(c, guardFor(okCfg())).WithClock(func() time.Time { return testNow }).PlanClusterRestart(context.Background(), "us-east-1", 24*time.Hour)
	if err != nil || len(plan) != 1 || plan[0].ID != "soon" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if len(rec.Calls()) != 1 {
		t.Fatalf("plan must only describe, calls=%d", len(rec.Calls()))
	}
}
