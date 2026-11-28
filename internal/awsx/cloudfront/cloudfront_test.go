package cloudfront

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
)

func TestListGetAndInvalidate(t *testing.T) {
	c, rec := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		switch {
		case call.Method == "POST":
			return awsxtest.Reply{Status: 201, Body: `<Invalidation><Id>I1</Id><Status>InProgress</Status><CreateTime>2026-11-21T12:00:00Z</CreateTime></Invalidation>`}
		case strings.HasSuffix(call.Path, "/invalidation"):
			return awsxtest.Reply{Body: `<InvalidationList><Items><InvalidationSummary><Id>I1</Id><Status>Completed</Status></InvalidationSummary></Items></InvalidationList>`}
		case strings.HasSuffix(call.Path, "/E1"):
			return awsxtest.Reply{Body: `<Distribution><Id>E1</Id><DomainName>d.cloudfront.net</DomainName><DistributionConfig><Comment>site</Comment><Enabled>true</Enabled></DistributionConfig></Distribution>`}
		}
		return awsxtest.Reply{Body: `<DistributionList><Items><DistributionSummary><Id>E1</Id><Status>Deployed</Status><DomainName>d.cloudfront.net</DomainName><Enabled>true</Enabled><Aliases><Items><CNAME>a.example.com</CNAME></Items></Aliases></DistributionSummary></Items></DistributionList>`}
	})
	cl := New(c)
	cl.host = ""
	page, err := cl.ListDistributions(context.Background(), "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Aliases[0] != "a.example.com" {
		t.Fatalf("list: %v %+v", err, page)
	}
	d, err := cl.GetDistribution(context.Background(), "E1")
	if err != nil || d.Comment != "site" || !d.Enabled {
		t.Fatalf("get: %v %+v", err, d)
	}
	inv, err := cl.CreateInvalidation(context.Background(), "E1", []string{"/a&b/*"})
	if err != nil || inv.ID != "I1" || inv.CreatedAt.IsZero() {
		t.Fatalf("invalidate: %v %+v", err, inv)
	}
	calls := rec.Calls()
	if body := calls[2].Body; !strings.Contains(body, "<Path>/a&amp;b/*</Path>") || !strings.Contains(body, "<Quantity>1</Quantity>") {
		t.Fatalf("body: %s", body)
	}
	list, _, err := cl.ListInvalidations(context.Background(), "E1", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("invalidations: %v %+v", err, list)
	}
}

func TestCleanPaths(t *testing.T) {
	got, err := CleanPaths([]string{"/a", " /a ", "/b"}, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("clean: %v %v", err, got)
	}
	for _, bad := range [][]string{nil, {"a"}, {"/a", "/b", "/c"}} {
		if _, err := CleanPaths(bad, 2); !errors.Is(err, ErrInvalidPaths) {
			t.Fatalf("%v accepted", bad)
		}
	}
}
