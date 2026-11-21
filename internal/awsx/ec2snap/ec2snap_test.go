package ec2snap

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
)

var testNow = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func guardFor() *cloudguard.Guard {
	cfg := cloudguard.Config{DefaultTTL: 24 * time.Hour, Now: func() time.Time { return testNow }}
	return cloudguard.New(func() cloudguard.Config { return cfg }, nil)
}

func form(c awsxtest.Call) url.Values { v, _ := url.ParseQuery(c.Body); return v }

func snapXML(id, vol, start string, managed bool) string {
	tag := ""
	if managed {
		tag = `<tagSet><item><key>levelrail:managed</key><value>true</value></item></tagSet>`
	}
	return `<item><snapshotId>` + id + `</snapshotId><volumeId>` + vol + `</volumeId><status>completed</status><volumeSize>8</volumeSize><startTime>` + start + `</startTime>` + tag + `</item>`
}

func TestDescribeSnapshotsPaginates(t *testing.T) {
	c, rec := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			return awsxtest.Reply{Body: `<R><nextToken>t1</nextToken><snapshotSet>` + snapXML("snap-a", "vol-1", "2026-11-20T01:00:00Z", true) + `</snapshotSet></R>`}
		}
		return awsxtest.Reply{Body: `<R><snapshotSet>` + snapXML("snap-b", "vol-1", "2026-11-19T01:00:00Z", false) + `</snapshotSet></R>`}
	})
	list, err := New(c, guardFor()).DescribeSnapshots(context.Background(), "us-east-1", "vol-1")
	if err != nil || len(list) != 2 || !list[0].Managed() || list[1].Managed() || list[0].SizeGB != 8 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	f := form(rec.Calls()[1])
	if f.Get("NextToken") != "t1" || f.Get("Owner.1") != "self" || f.Get("Filter.1.Name") != "volume-id" || f.Get("Action") != "DescribeSnapshots" {
		t.Fatalf("second page form = %v", f)
	}
}

func TestCreateSnapshotsTagsAndExpires(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `<CreateSnapshotsResponse><snapshotSet>` + snapXML("snap-1", "vol-1", "2026-11-21T12:00:00Z", true) + snapXML("snap-2", "vol-2", "2026-11-21T12:00:00Z", true) + `</snapshotSet></CreateSnapshotsResponse>`}
	})
	got, err := New(c, guardFor()).CreateSnapshots(context.Background(), CreateInput{Region: "us-east-1", InstanceID: "i-1", ExcludeBoot: true, Meta: Meta{Owner: "o", Env: "e", Project: "p", ID: "i-1"}})
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	f := form(rec.Calls()[0])
	if f.Get("Action") != "CreateSnapshots" || f.Get("InstanceSpecification.InstanceId") != "i-1" || f.Get("InstanceSpecification.ExcludeBootVolume") != "true" || f.Get("TagSpecification.1.ResourceType") != "snapshot" {
		t.Fatalf("form = %v", f)
	}
	keys := map[string]string{}
	for i := 1; i < 20; i++ {
		if k := f.Get("TagSpecification.1.Tag." + strconv.Itoa(i) + ".Key"); k != "" {
			keys[k] = f.Get("TagSpecification.1.Tag." + strconv.Itoa(i) + ".Value")
		}
	}
	if keys["levelrail:managed"] != "true" || keys["levelrail:expires"] != "2026-11-22T12:00:00Z" {
		t.Fatalf("tags = %v", keys)
	}
	if _, err := New(c, guardFor()).CreateSnapshots(context.Background(), CreateInput{Region: "us-east-1"}); !errors.Is(err, ErrNoSource) {
		t.Fatalf("no source err = %v", err)
	}
}

func TestDeleteRefusesUnmanaged(t *testing.T) {
	managed := false
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `<R><snapshotSet>` + snapXML("snap-a", "vol-1", "2026-11-20T01:00:00Z", managed) + `</snapshotSet></R>`}
	})
	cl := New(c, guardFor())
	if err := cl.DeleteSnapshot(context.Background(), "us-east-1", "snap-a"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("err = %v", err)
	}
	for _, call := range rec.Calls() {
		if form(call).Get("Action") == "DeleteSnapshot" {
			t.Fatal("unmanaged snapshot was deleted")
		}
	}
	managed = true
	if err := cl.DeleteSnapshot(context.Background(), "us-east-1", "snap-a"); err != nil {
		t.Fatal(err)
	}
	last := rec.Calls()[len(rec.Calls())-1]
	if form(last).Get("Action") != "DeleteSnapshot" {
		t.Fatalf("last call = %s", last.Body)
	}
}

func TestPlanRetentionTable(t *testing.T) {
	day := func(d int) time.Time { return testNow.AddDate(0, 0, -d) }
	mk := func(id, vol string, d int, managed bool) Snapshot {
		tags := map[string]string{}
		if managed {
			tags[ManagedTag] = "true"
		}
		return Snapshot{ID: id, VolumeID: vol, StartTime: day(d), Tags: tags}
	}
	var many []Snapshot
	for d := 0; d < 10; d++ {
		many = append(many, mk("m"+strconv.Itoa(d), "vol-1", d, true))
	}
	cases := []struct {
		name string
		in   []Snapshot
		r    Retention
		want int
	}{
		{"no policy keeps all", many, Retention{}, 0},
		{"keep 3 daily no weekly", many, Retention{Daily: 3}, 7},
		{"unmanaged never listed", []Snapshot{mk("u1", "vol-1", 50, false), mk("u2", "vol-1", 60, false)}, Retention{Daily: 1}, 0},
		{"two volumes independent", []Snapshot{mk("a1", "vol-a", 1, true), mk("a2", "vol-a", 2, true), mk("b1", "vol-b", 1, true), mk("b2", "vol-b", 2, true)}, Retention{Daily: 1}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanRetention(tc.in, tc.r)
			if len(got) != tc.want {
				t.Fatalf("got %d deletions: %+v", len(got), got)
			}
			for _, d := range got {
				if d.Reason != ReasonExpired || d.ID[0] == 'u' {
					t.Fatalf("bad deletion %+v", d)
				}
			}
		})
	}
	// The newest snapshot must always survive a daily policy.
	for _, d := range PlanRetention(many, Retention{Daily: 3}) {
		if d.ID == "m0" || d.ID == "m1" || d.ID == "m2" {
			t.Fatalf("kept snapshot listed: %+v", d)
		}
	}
}
