package secrets

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
)

func TestGetParametersByPath(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"Parameters":[{"Name":"/app/db","Type":"SecureString","Value":"hunter2"}]}`}
	})
	got, err := New(c).GetParametersByPath(context.Background(), "us-east-1", "/app", true)
	call := rec.Calls()[0]
	if err != nil || len(got) != 1 || got[0].Value.Reveal() != "hunter2" || call.Target != "AmazonSSM.GetParametersByPath" ||
		!strings.Contains(call.Body, `"WithDecryption":true`) {
		t.Fatalf("got %v %+v %+v", err, got, call)
	}
	if strings.Contains(fmt.Sprintf("%v %+v %#v", got[0], got[0], got[0]), "hunter2") {
		t.Fatal("value printed")
	}
}

func TestPutParameterSendsRealValueButErrorIsScrubbed(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Status: 400, Header: map[string]string{"X-Amzn-Errortype": "ParameterAlreadyExists"}, Body: `{"message":"hunter2"}`}
	})
	_, err := New(c).PutParameter(context.Background(), "us-east-1", "/app/db", "hunter2", false)
	if !strings.Contains(rec.Calls()[0].Body, `"Value":"hunter2"`) || !strings.Contains(rec.Calls()[0].Body, `"Type":"SecureString"`) {
		t.Fatalf("body %s", rec.Calls()[0].Body)
	}
	if err == nil || strings.Contains(err.Error(), "hunter2") || awsx.ClassOf(err) != awsx.ClassConflict {
		t.Fatalf("got %v", err)
	}
}

func TestSecretsManager(t *testing.T) {
	c, rec := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		switch {
		case strings.HasSuffix(call.Target, "ListSecrets"):
			return awsxtest.Reply{Body: `{"SecretList":[{"Name":"n","ARN":"a"}]}`}
		case strings.HasSuffix(call.Target, "GetSecretValue"):
			return awsxtest.Reply{Body: `{"SecretString":"s3cret"}`}
		}
		return awsxtest.Reply{Body: `{}`}
	})
	m := New(c)
	l, err := m.ListSecrets(context.Background(), "us-east-1")
	v, err2 := m.GetSecretValue(context.Background(), "us-east-1", "n")
	err3 := m.PutSecretValue(context.Background(), "us-east-1", "n", "new")
	if err != nil || err2 != nil || err3 != nil || len(l) != 1 || v.Reveal() != "s3cret" {
		t.Fatalf("got %v %v %v", err, err2, err3)
	}
	if rec.Calls()[0].Target != "secretsmanager.ListSecrets" || !strings.Contains(rec.Calls()[2].Body, `"SecretString":"new"`) {
		t.Fatalf("calls %+v", rec.Calls())
	}
}

func TestRedact(t *testing.T) {
	if got := Redact("a hunter2 b", "hunter2", ""); got != "a [redacted] b" {
		t.Fatal(got)
	}
}
