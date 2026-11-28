package ssm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
)

const iid = "i-0123456789abcdef0"

func TestSendCommandAllowlist(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"Command":{"CommandId":"11111111-2222-3333-4444-555555555555"}}`}
	})
	cl := New(c)
	if _, err := cl.SendCommand(context.Background(), "us-east-1", iid, "uptime", nil, 60); !errors.Is(err, ErrRefused) {
		t.Fatal("empty allowlist must refuse")
	}
	if _, err := cl.SendCommand(context.Background(), "us-east-1", "i-1; rm", "uptime", []string{"uptime"}, 60); !errors.Is(err, ErrRefused) {
		t.Fatal("bad instance id accepted")
	}
	if len(rec.Calls()) != 0 {
		t.Fatal("refused calls must not reach AWS")
	}
	id, err := cl.SendCommand(context.Background(), "us-east-1", iid, " uptime ", []string{"uptime"}, 60)
	if err != nil || id == "" {
		t.Fatalf("send: %v", err)
	}
	b := rec.Calls()[0]
	if !strings.HasSuffix(b.Target, "SendCommand") || !strings.Contains(b.Body, "AWS-RunShellScript") || !strings.Contains(b.Body, `"commands":["uptime"]`) {
		t.Fatalf("body: %+v", b)
	}
}

func TestDescribeInvocationSession(t *testing.T) {
	c, _ := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		switch {
		case strings.HasSuffix(call.Target, "DescribeInstanceInformation"):
			return awsxtest.Reply{Body: `{"InstanceInformationList":[{"InstanceId":"` + iid + `","PingStatus":"Online","LastPingDateTime":1.7e9}]}`}
		case strings.HasSuffix(call.Target, "GetCommandInvocation"):
			return awsxtest.Reply{Body: `{"Status":"Success","ResponseCode":0,"StandardOutputContent":"ok"}`}
		}
		return awsxtest.Reply{Body: `{"SessionId":"s1","TokenValue":"tok","StreamUrl":"wss://x"}`}
	})
	cl := New(c)
	list, err := cl.DescribeInstances(context.Background(), "us-east-1", "")
	if err != nil || len(list) != 1 || list[0].PingStatus != "Online" || list[0].LastPing.IsZero() {
		t.Fatalf("describe: %v %+v", err, list)
	}
	inv, err := cl.GetInvocation(context.Background(), "us-east-1", "11111111-2222-3333-4444-555555555555", iid)
	if err != nil || inv.Stdout != "ok" {
		t.Fatalf("invocation: %v %+v", err, inv)
	}
	s, err := cl.StartSession(context.Background(), "us-east-1", iid)
	if err != nil || s.Token != "tok" {
		t.Fatalf("session: %v %+v", err, s)
	}
}
