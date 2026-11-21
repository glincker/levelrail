package ecr

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
)

func guard(t *testing.T, kill bool) *cloudguard.Guard {
	t.Helper()
	return cloudguard.New(func() cloudguard.Config {
		return cloudguard.Config{KillSwitch: kill, ServicesEnabled: map[string]bool{ServiceName: true}}
	}, nil)
}

var meta = cloudguard.TagMeta{Owner: "o", Env: "e", Project: "p", ID: "i"}

func TestAuthToken(t *testing.T) {
	tok := base64.StdEncoding.EncodeToString([]byte("AWS:pw:with:colon"))
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"authorizationData":[{"authorizationToken":"` + tok + `","expiresAt":1.7e9,"proxyEndpoint":"https://1.dkr.ecr.us-east-1.amazonaws.com"}]}`}
	})
	got, err := New(c, guard(t, false), Options{}).GetAuthorizationToken(context.Background(), "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Registry != "1.dkr.ecr.us-east-1.amazonaws.com" || got.User != "AWS" || got.Password.Reveal() != "pw:with:colon" || got.ExpiresAt.IsZero() {
		t.Fatalf("bad token %+v", got)
	}
	if rec.Calls()[0].Target != "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken" {
		t.Fatal("wrong target")
	}
}

func TestCreateRepositoryFlow(t *testing.T) {
	c, rec := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		if strings.HasSuffix(call.Target, "CreateRepository") {
			return awsxtest.Reply{Body: `{"repository":{"repositoryName":"app","repositoryUri":"u","repositoryArn":"a"}}`}
		}
		return awsxtest.Reply{Body: `{}`}
	})
	repo, err := New(c, guard(t, false), Options{UntaggedExpireDays: 7, KeepTaggedImages: 30}).
		CreateRepository(context.Background(), CreateInput{Account: "acc", Region: "us-east-1", Name: "app", Meta: meta})
	if err != nil || !repo.ScanOnPush {
		t.Fatalf("create: %v %+v", err, repo)
	}
	calls := rec.Calls()
	if len(calls) != 2 || !strings.Contains(calls[0].Body, `"scanOnPush":true`) || !strings.Contains(calls[0].Body, "managed") {
		t.Fatalf("create body: %+v", calls)
	}
	if !strings.HasSuffix(calls[1].Target, "PutLifecyclePolicy") || !strings.Contains(calls[1].Body, `\"rulePriority\":2`) && !strings.Contains(calls[1].Body, "rulePriority") {
		t.Fatalf("policy call: %+v", calls[1])
	}
}

func TestCreateRefusedMakesNoCall(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: `{}`} })
	_, err := New(c, guard(t, true), Options{}).CreateRepository(context.Background(), CreateInput{Account: "a", Region: "r", Name: "x", Meta: meta})
	if !errors.Is(err, ErrRefused) || len(rec.Calls()) != 0 {
		t.Fatalf("want refusal with no call, got %v", err)
	}
}

func TestDeleteRefusedWhenNotEmpty(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"imageIds":[{"imageTag":"v1"}]}`}
	})
	err := New(c, guard(t, false), Options{}).DeleteRepository(context.Background(), "us-east-1", "app")
	if !errors.Is(err, ErrNotEmpty) || len(rec.Calls()) != 1 {
		t.Fatalf("want not empty, got %v", err)
	}
}

func TestDeleteEmptyAndErrorClass(t *testing.T) {
	c, rec := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		if strings.HasSuffix(call.Target, "ListImages") {
			return awsxtest.Reply{Body: `{"imageIds":[]}`}
		}
		return awsxtest.Reply{Status: 400, Header: map[string]string{"X-Amzn-Errortype": "RepositoryNotFoundException:"}, Body: `{"message":"no"}`}
	})
	err := New(c, guard(t, false), Options{}).DeleteRepository(context.Background(), "us-east-1", "app")
	if awsx.ClassOf(err) != awsx.ClassNotFound || awsx.CodeOf(err) != "RepositoryNotFoundException" || len(rec.Calls()) != 2 {
		t.Fatalf("got %v", err)
	}
}

func TestDescribeRepositoriesPages(t *testing.T) {
	c, _ := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			return awsxtest.Reply{Body: `{"repositories":[{"repositoryName":"a"}],"nextToken":"t"}`}
		}
		return awsxtest.Reply{Body: `{"repositories":[{"repositoryName":"b","imageScanningConfiguration":{"scanOnPush":true}}]}`}
	})
	got, err := New(c, guard(t, false), Options{}).DescribeRepositories(context.Background(), "us-east-1")
	if err != nil || len(got) != 2 || !got[1].ScanOnPush {
		t.Fatalf("got %v %+v", err, got)
	}
}
