package provision

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestAWSCreateWithoutGuardRefusesAndCallsNothing(t *testing.T) {
	fake := defaultCreateFixture()
	a := NewAWS(AWSCredentials{AccessKeyID: "ak", SecretAccessKey: "sk"}, "app")
	a.client = func(context.Context, string) (ec2API, error) { return fake, nil }
	_, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.micro", Name: "n"})
	var ref *GuardRefusal
	if !errors.As(err, &ref) || ref.Code != "guard_missing" {
		t.Fatalf("err = %v, want guard_missing refusal", err)
	}
	if fake.runCalls != 0 || fake.createSGCalls != 0 {
		t.Fatalf("aws was called: run=%d sg=%d", fake.runCalls, fake.createSGCalls)
	}
}

func TestAWSCreateRefusedByGuardMakesNoCall(t *testing.T) {
	fake := defaultCreateFixture()
	a := newTestAWS(fake)
	a.SetCreateGuard(func(context.Context, EC2CreateRequest) (map[string]string, error) {
		return nil, &GuardRefusal{Code: "kill_switch", Message: "paused"}
	})
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.micro", Name: "n"}); err == nil {
		t.Fatal("want refusal")
	}
	if fake.runCalls != 0 || fake.createSGCalls != 0 {
		t.Fatalf("aws was called: run=%d sg=%d", fake.runCalls, fake.createSGCalls)
	}
}

func TestAWSCreateAllowedCarriesAllTags(t *testing.T) {
	fake := defaultCreateFixture()
	a := newTestAWS(fake)
	a.SetCreateGuard(func(_ context.Context, r EC2CreateRequest) (map[string]string, error) {
		return map[string]string{"Name": r.Name, "levelrail:managed": "true", "levelrail:owner": "o"}, nil
	})
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.micro", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	specs := fake.runIn.TagSpecifications
	want := []types.ResourceType{types.ResourceTypeInstance, types.ResourceTypeVolume, types.ResourceTypeNetworkInterface}
	if len(specs) != len(want) {
		t.Fatalf("specs = %d", len(specs))
	}
	for i, s := range specs {
		got := map[string]string{}
		for _, tg := range s.Tags {
			got[aws.ToString(tg.Key)] = aws.ToString(tg.Value)
		}
		if s.ResourceType != want[i] || got["levelrail:managed"] != "true" || got["levelrail:owner"] != "o" || got["Name"] != "n" || got[awsManagedTagKey] == "" {
			t.Fatalf("spec %d = %v", i, got)
		}
	}
}
