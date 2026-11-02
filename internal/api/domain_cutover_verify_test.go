package api

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cutoververify"
)

type verifyFakeResolver struct{ hosts map[string][]string }

func (f verifyFakeResolver) LookupHost(_ context.Context, n string) ([]string, error) {
	if h, ok := f.hosts[n]; ok {
		return h, nil
	}
	return nil, errors.New("no such host")
}

func (verifyFakeResolver) LookupCNAME(_ context.Context, n string) (string, error) { return n, nil }

type verifyFakeProber struct{ calls int }

func (p *verifyFakeProber) HTTP(_ context.Context, u string) (cutoververify.HTTPResult, error) {
	p.calls++
	if u == "http://far.example.com/" {
		return cutoververify.HTTPResult{Status: 301, Location: "https://far.example.com/"}, nil
	}
	return cutoververify.HTTPResult{Status: 200, TTFB: 10 * time.Millisecond, Header: map[string]string{"Strict-Transport-Security": "max-age=31536000"}}, nil
}

func (p *verifyFakeProber) Cert(context.Context, string) (cutoververify.CertInfo, error) {
	p.calls++
	return cutoververify.CertInfo{Issuer: "R11", NotAfter: time.Now().Add(60 * 24 * time.Hour), Trusted: true}, nil
}

func TestDomainCutoverVerify(t *testing.T) {
	rt, cookie := cutoverRouter(t, true, new(atomic.Int32))
	p := &verifyFakeProber{}
	rt.domainAuto.verify = verifySeams{prober: p, resolver: verifyFakeResolver{hosts: map[string][]string{"far.example.com": {"198.51.100.4"}, "www.far.example.com": {"198.51.100.4"}}}}

	var rep cutoververify.Report
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/verify", &rep); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if rep.Verdict != cutoververify.Fail || p.calls != 0 {
		t.Fatalf("still-elsewhere domain: verdict %s, probe calls %d", rep.Verdict, p.calls)
	}

	rep = cutoververify.Report{}
	trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/verify?expect_ip=198.51.100.4", &rep)
	if rep.Verdict != cutoververify.Pass || p.calls == 0 {
		t.Fatalf("matching domain: verdict %s lines %+v", rep.Verdict, rep.Lines)
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/verify?expect_ip=nope", nil); code != http.StatusBadRequest {
		t.Errorf("bad expect_ip status = %d, want 400", code)
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/nobody.example.com/verify", nil); code != http.StatusNotFound {
		t.Errorf("unknown domain status = %d, want 404", code)
	}
}
