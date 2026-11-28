package lightsail

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/cloudguard"
)

func guard(maxUSD float64) *cloudguard.Guard {
	return cloudguard.New(func() cloudguard.Config {
		return cloudguard.Config{ServicesEnabled: map[string]bool{ServiceName: true}, DefaultTTL: time.Hour, Now: time.Now, PerCreateMaxUSD: maxUSD}
	}, nil)
}

func instanceJSON(managed bool) string {
	tags := `[]`
	if managed {
		tags = `[{"key":"` + ManagedTag + `","value":"true"}]`
	}
	return `{"name":"web","arn":"arn:i","createdAt":1700000000.5,"bundleId":"nano_3_0","publicIpAddress":"1.2.3.4",` +
		`"location":{"availabilityZone":"us-east-1a","regionName":"us-east-1"},"hardware":{"cpuCount":2,"ramSizeInGb":0.5,"disks":[{"sizeInGb":20}]},` +
		`"state":{"name":"running"},"tags":` + tags + `}`
}

const bundlesJSON = `{"bundles":[{"bundleId":"nano_3_0","price":5,"cpuCount":2,"isActive":true},{"bundleId":"big","price":80,"isActive":true}]}`

func fake(managed bool, count int) func(awsxtest.Call, int) awsxtest.Reply {
	return func(c awsxtest.Call, _ int) awsxtest.Reply {
		switch strings.TrimPrefix(c.Target, Service.TargetPrefix+".") {
		case "GetInstance":
			return awsxtest.Reply{Body: `{"instance":` + instanceJSON(managed) + `}`}
		case "GetInstances":
			items := make([]string, count)
			for i := range items {
				items[i] = instanceJSON(true)
			}
			return awsxtest.Reply{Body: `{"instances":[` + strings.Join(items, ",") + `]}`}
		case "GetBundles":
			return awsxtest.Reply{Body: bundlesJSON}
		}
		return awsxtest.Reply{Body: `{}`}
	}
}

func sent(rec *awsxtest.Recorder, op string) bool {
	for _, c := range rec.Calls() {
		if c.Target == Service.TargetPrefix+"."+op {
			return true
		}
	}
	return false
}

func TestGetInstanceReadsManagedTag(t *testing.T) {
	c, _ := awsxtest.New(t, fake(true, 0))
	i, err := New(c, guard(0)).GetInstance(context.Background(), "us-east-1", "web")
	if err != nil || !i.Managed || i.CPUs != 2 || i.DiskGB != 20 || i.CreatedAt.Unix() != 1700000000 || i.State != "running" {
		t.Fatalf("i=%+v err=%v", i, err)
	}
}

func TestChangesNeedManagedTag(t *testing.T) {
	c, rec := awsxtest.New(t, fake(false, 0))
	cl := New(c, guard(0))
	ctx := context.Background()
	for name, err := range map[string]error{
		"start":  cl.Start(ctx, "r", "web"),
		"stop":   cl.Stop(ctx, "r", "web"),
		"reboot": cl.Reboot(ctx, "r", "web"),
		"delete": cl.Delete(ctx, DeleteInput{Region: "r", Name: "web", Confirm: "web"}),
		"snap":   cl.CreateSnapshot(ctx, "r", "web", "s", Meta{}),
	} {
		if !errors.Is(err, ErrNotManaged) {
			t.Errorf("%s err=%v", name, err)
		}
	}
	for _, op := range []string{"StartInstance", "StopInstance", "RebootInstance", "DeleteInstance", "CreateInstanceSnapshot"} {
		if sent(rec, op) {
			t.Errorf("%s reached AWS", op)
		}
	}
}

func TestDeleteNeedsTypedConfirm(t *testing.T) {
	c, rec := awsxtest.New(t, fake(true, 0))
	cl := New(c, guard(0))
	if err := cl.Delete(context.Background(), DeleteInput{Region: "r", Name: "web", Confirm: "no"}); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("err=%v", err)
	}
	if len(rec.Calls()) != 0 {
		t.Fatal("a refused delete made a call")
	}
	if err := cl.Delete(context.Background(), DeleteInput{Region: "r", Name: "web", Confirm: "web"}); err != nil || !sent(rec, "DeleteInstance") {
		t.Fatalf("err=%v", err)
	}
}

func createIn() CreateInput {
	return CreateInput{Account: "a", Region: "us-east-1", Name: "n1", Zone: "us-east-1a", Blueprint: "ubuntu_22_04", Bundle: "nano_3_0",
		Meta: Meta{Owner: "o", Env: "e", Project: "p", ID: "n1"}, AllowedBundles: []string{"nano_3_0", "big"}, MaxManaged: 2}
}

func TestCreateGates(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(*CreateInput) (*cloudguard.Guard, int){
		"empty allowlist": func(in *CreateInput) (*cloudguard.Guard, int) { in.AllowedBundles = nil; return guard(0), 0 },
		"bundle off list": func(in *CreateInput) (*cloudguard.Guard, int) { in.Bundle = "other"; return guard(0), 0 },
		"per create max":  func(in *CreateInput) (*cloudguard.Guard, int) { in.Bundle = "big"; return guard(10), 0 },
		"hard max":        func(*CreateInput) (*cloudguard.Guard, int) { return guard(0), 2 },
	}
	for name, mod := range cases {
		in := createIn()
		g, n := mod(&in)
		c, rec := awsxtest.New(t, fake(true, n))
		if err := New(c, g).Create(ctx, in); !errors.Is(err, ErrRefused) {
			t.Errorf("%s err=%v", name, err)
		}
		if sent(rec, "CreateInstances") {
			t.Errorf("%s reached AWS", name)
		}
	}
}

func TestCreateSendsTags(t *testing.T) {
	c, rec := awsxtest.New(t, fake(true, 0))
	if err := New(c, guard(0)).Create(context.Background(), createIn()); err != nil {
		t.Fatal(err)
	}
	var body string
	for _, call := range rec.Calls() {
		if call.Target == Service.TargetPrefix+".CreateInstances" {
			body = call.Body
		}
	}
	for _, want := range []string{ManagedTag, tagPrefix + "owner", tagPrefix + "env", tagPrefix + "expires", `"instanceNames":["n1"]`} {
		if !strings.Contains(body, want) {
			t.Errorf("create body lacks %s: %s", want, body)
		}
	}
}
