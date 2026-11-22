package awsx_test

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
)

var svc = awsx.Service{Name: "x", Prefix: "x", TargetPrefix: "T"}

func TestRetryOnThrottleThenSuccess(t *testing.T) {
	c, rec := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n < 3 {
			return awsxtest.Reply{Status: 400, Header: map[string]string{"X-Amzn-Errortype": "ThrottlingException"}, Body: `{}`}
		}
		return awsxtest.Reply{Body: `{"ok":true}`}
	})
	var out struct{ OK bool }
	if err := c.JSON(context.Background(), svc, "us-east-1", "Op", map[string]string{}, &out); err != nil || !out.OK {
		t.Fatalf("got %v %+v", err, out)
	}
	if n := len(rec.Calls()); n != 3 {
		t.Fatalf("want 3 attempts, got %d", n)
	}
	if !strings.Contains(rec.Calls()[0].Auth, "AWS4-HMAC-SHA256") || rec.Calls()[0].Target != "T.Op" {
		t.Fatal("request not signed or wrong target")
	}
}

func TestNoRetryOnAuthAndClasses(t *testing.T) {
	cases := []struct {
		status int
		code   string
		class  awsx.Class
	}{
		{403, "AccessDeniedException", awsx.ClassAuth},
		{400, "ValidationException", awsx.ClassInvalid},
		{400, "ResourceNotFoundException", awsx.ClassNotFound},
		{400, "RepositoryAlreadyExistsException", awsx.ClassConflict},
	}
	for _, tc := range cases {
		c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
			return awsxtest.Reply{Status: tc.status, Body: `{"__type":"com.amazonaws#` + tc.code + `","message":"secret-value"}`}
		})
		err := c.JSON(context.Background(), svc, "us-east-1", "Op", struct{}{}, nil)
		if awsx.ClassOf(err) != tc.class || awsx.CodeOf(err) != tc.code || len(rec.Calls()) != 1 {
			t.Fatalf("%s: got %v", tc.code, err)
		}
		if strings.Contains(err.Error(), "secret-value") {
			t.Fatal("error text leaked the message")
		}
	}
}

func TestQueryXMLErrorAndRetryServer(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Status: 503, Body: `<Response><Errors><Error><Code>ServiceUnavailable</Code><Message>m</Message></Error></Errors></Response>`}
	})
	err := c.Query(context.Background(), svc, "us-east-1", "DescribeX", "2016-11-15", nil, nil)
	if awsx.ClassOf(err) != awsx.ClassServer || len(rec.Calls()) != 3 {
		t.Fatalf("got %v after %d calls", err, len(rec.Calls()))
	}
	if !strings.Contains(rec.Calls()[0].Body, "Action=DescribeX") {
		t.Fatal("missing action")
	}
}

func TestEndpointAndCreds(t *testing.T) {
	if got := svc.Endpoint("cn-north-1"); got != "https://x.cn-north-1.amazonaws.com.cn" {
		t.Fatal(got)
	}
	if _, err := awsx.FromMap(map[string]string{"access_key_id": "a"}); err == nil {
		t.Fatal("want incomplete error")
	}
	if awsx.Secret("v").String() != "[redacted]" {
		t.Fatal("secret printed")
	}
}
