package cloudtrail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/awsx"
	"github.com/glincker/levelrail/internal/awsx/awsxtest"
)

func TestLookupEventsShapeAndOrder(t *testing.T) {
	c, rec := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			return awsxtest.Reply{Body: `{"Events":[{"EventId":"1","EventName":"RunInstances","EventTime":1.7e9,"Username":"u","Resources":[{"ResourceName":"i-1"}]}],"NextToken":"t"}`}
		}
		return awsxtest.Reply{Body: `{"Events":[{"EventId":"2","EventName":"StopInstances","EventTime":1.8e9}]}`}
	})
	got, err := New(c).LookupEvents(context.Background(), Query{
		Region: "us-east-1", AttrKey: AttrResourceName, AttrValue: "i-1", Start: time.Unix(1, 0), MaxPages: 2,
	})
	call := rec.Calls()[0]
	if err != nil || len(got) != 2 || got[0].ID != "2" || got[1].Resources[0] != "i-1" {
		t.Fatalf("got %v %+v", err, got)
	}
	if !strings.HasSuffix(call.Target, "CloudTrail_20131101.LookupEvents") ||
		!strings.Contains(call.Body, `"AttributeKey":"ResourceName"`) || !strings.Contains(call.Body, `"StartTime":1`) {
		t.Fatalf("call %+v", call)
	}
}

func TestLookupNeedsAttribute(t *testing.T) {
	c, _ := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: `{}`} })
	if _, err := New(c).LookupEvents(context.Background(), Query{Region: "r"}); err == nil {
		t.Fatal("want error")
	}
}

func TestLookupByNamesDedupesAndClassifies(t *testing.T) {
	c, _ := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		if strings.Contains(call.Body, "bad") {
			return awsxtest.Reply{Status: 400, Header: map[string]string{"X-Amzn-Errortype": "InvalidLookupAttributesException"}, Body: `{}`}
		}
		return awsxtest.Reply{Body: `{"Events":[{"EventId":"1","EventTime":1.7e9}]}`}
	})
	m := New(c)
	got, err := m.LookupByNames(context.Background(), Query{Region: "r"}, []string{"a", "b"})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v %v", err, got)
	}
	_, err = m.LookupByNames(context.Background(), Query{Region: "r"}, []string{"bad"})
	if awsx.ClassOf(err) != awsx.ClassInvalid {
		t.Fatalf("got %v", err)
	}
}
