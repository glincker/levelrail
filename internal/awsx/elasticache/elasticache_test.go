package elasticache

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/cloudguard"
)

func guard() *cloudguard.Guard {
	return cloudguard.New(func() cloudguard.Config {
		return cloudguard.Config{ServicesEnabled: map[string]bool{ServiceName: true}, DefaultTTL: time.Hour, Now: time.Now}
	}, nil)
}

func form(c awsxtest.Call) url.Values { v, _ := url.ParseQuery(c.Body); return v }

const groupXML = `<R><DescribeReplicationGroupsResult><ReplicationGroups><ReplicationGroup><ReplicationGroupId>rg1</ReplicationGroupId><ARN>arn:rg1</ARN><Status>available</Status><NodeGroups><NodeGroup><PrimaryEndpoint><Address>h</Address><Port>6379</Port></PrimaryEndpoint></NodeGroup></NodeGroups></ReplicationGroup></ReplicationGroups></DescribeReplicationGroupsResult></R>`

func tagsXML(managed bool) string {
	if !managed {
		return `<R><ListTagsForResourceResult><TagList/></ListTagsForResourceResult></R>`
	}
	return `<R><ListTagsForResourceResult><TagList><Tag><Key>` + ManagedTag + `</Key><Value>true</Value></Tag></TagList></ListTagsForResourceResult></R>`
}

func fake(managed bool) func(awsxtest.Call, int) awsxtest.Reply {
	return func(c awsxtest.Call, _ int) awsxtest.Reply {
		switch form(c).Get("Action") {
		case "DescribeReplicationGroups":
			return awsxtest.Reply{Body: groupXML}
		case "ListTagsForResource":
			return awsxtest.Reply{Body: tagsXML(managed)}
		}
		return awsxtest.Reply{Body: "<R/>"}
	}
}

func TestDescribeReadsManagedTag(t *testing.T) {
	c, _ := awsxtest.New(t, fake(true))
	list, err := New(c, guard()).DescribeReplicationGroups(context.Background(), "us-east-1")
	if err != nil || len(list) != 1 || !list[0].Managed || list[0].Port != 6379 || list[0].PrimaryEndpoint != "h" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

func TestDeleteNeedsConfirmAndManaged(t *testing.T) {
	c, rec := awsxtest.New(t, fake(false))
	cl := New(c, guard())
	if err := cl.DeleteReplicationGroup(context.Background(), DeleteInput{Region: "r", ID: "rg1", Confirm: "no"}); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("confirm err=%v", err)
	}
	if err := cl.DeleteReplicationGroup(context.Background(), DeleteInput{Region: "r", ID: "rg1", Confirm: "rg1"}); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("unmanaged err=%v", err)
	}
	for _, call := range rec.Calls() {
		if strings.HasPrefix(form(call).Get("Action"), "Delete") {
			t.Fatal("a refused delete reached AWS")
		}
	}
}

func TestDeleteManagedSendsFinalSnapshot(t *testing.T) {
	c, rec := awsxtest.New(t, fake(true))
	err := New(c, guard()).DeleteReplicationGroup(context.Background(), DeleteInput{Region: "r", ID: "rg1", Confirm: "rg1", FinalSnapshotID: "fin"})
	if err != nil {
		t.Fatal(err)
	}
	last := form(rec.Calls()[len(rec.Calls())-1])
	if last.Get("Action") != "DeleteReplicationGroup" || last.Get("FinalSnapshotIdentifier") != "fin" {
		t.Fatalf("last call = %v", last)
	}
}
