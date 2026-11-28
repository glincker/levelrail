package acm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx"
	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/cloudguard"
)

func guard(kill bool) *cloudguard.Guard {
	return cloudguard.New(func() cloudguard.Config {
		return cloudguard.Config{KillSwitch: kill, ServicesEnabled: map[string]bool{ServiceName: true}}
	}, nil)
}

var meta = cloudguard.TagMeta{Owner: "o", Env: "e", Project: "p", ID: "i"}

func TestRequestCertificate(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: `{"CertificateArn":"arn:1"}`} })
	arn, err := New(c, guard(false)).RequestCertificate(context.Background(),
		RequestInput{Account: "a", Region: "us-east-1", Domain: "x.dev", AltNames: []string{"*.x.dev"}, Meta: meta})
	call := rec.Calls()[0]
	if err != nil || arn != "arn:1" || call.Target != "CertificateManager.RequestCertificate" ||
		!strings.Contains(call.Body, `"ValidationMethod":"DNS"`) || !strings.Contains(call.Body, `"SubjectAlternativeNames":["*.x.dev"]`) {
		t.Fatalf("got %v %q %+v", err, arn, call)
	}
}

func TestRequestRefused(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: `{}`} })
	_, err := New(c, guard(true)).RequestCertificate(context.Background(), RequestInput{Account: "a", Region: "r", Domain: "x", Meta: meta})
	if !errors.Is(err, ErrRefused) || len(rec.Calls()) != 0 {
		t.Fatalf("got %v", err)
	}
}

func TestDescribeReturnsRecords(t *testing.T) {
	c, _ := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"Certificate":{"CertificateArn":"arn:1","DomainName":"x.dev","Status":"PENDING_VALIDATION","DomainValidationOptions":[
			{"DomainName":"x.dev","ValidationStatus":"PENDING_VALIDATION","ResourceRecord":{"Name":"_a.x.dev.","Type":"CNAME","Value":"_b.acm-validations.aws."}},
			{"DomainName":"y.dev","ValidationStatus":"PENDING_VALIDATION"}]}}`}
	})
	got, err := New(c, guard(false)).DescribeCertificate(context.Background(), "us-east-1", "arn:1")
	if err != nil || len(got.Records) != 1 || got.Records[0].Type != "CNAME" || got.Records[0].Name != "_a.x.dev." {
		t.Fatalf("got %v %+v", err, got)
	}
}

func TestListPagesAndErrorClass(t *testing.T) {
	c, _ := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			return awsxtest.Reply{Body: `{"CertificateSummaryList":[{"CertificateArn":"a"}],"NextToken":"t"}`}
		}
		if n == 2 {
			return awsxtest.Reply{Body: `{"CertificateSummaryList":[{"CertificateArn":"b"}]}`}
		}
		return awsxtest.Reply{Status: 400, Header: map[string]string{"X-Amzn-Errortype": "ResourceNotFoundException"}, Body: `{}`}
	})
	m := New(c, guard(false))
	got, err := m.ListCertificates(context.Background(), "us-east-1")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v %v", err, got)
	}
	_, err = m.DescribeCertificate(context.Background(), "us-east-1", "arn:x")
	if awsx.ClassOf(err) != awsx.ClassNotFound {
		t.Fatalf("got %v", err)
	}
}
