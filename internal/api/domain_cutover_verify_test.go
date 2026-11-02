package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func (p *verifyFakeProber) HTTP(_ context.Context, u string, _ []string) (cutoververify.HTTPResult, error) {
	p.calls++
	if u == "http://far.example.com/" {
		return cutoververify.HTTPResult{Status: 301, Location: "https://far.example.com/"}, nil
	}
	return cutoververify.HTTPResult{Status: 200, TTFB: 10 * time.Millisecond, Header: map[string]string{"Strict-Transport-Security": "max-age=31536000"}}, nil
}

func (p *verifyFakeProber) Cert(context.Context, string, []string) (cutoververify.CertInfo, error) {
	p.calls++
	return cutoververify.CertInfo{Issuer: "R11", NotAfter: time.Now().Add(60 * 24 * time.Hour), Trusted: true}, nil
}

func TestDomainCutoverVerify(t *testing.T) {
	rt, cookie := cutoverRouter(t, true, new(atomic.Int32))
	verifyLimit.reset()
	verifyNow = func() time.Time { return time.Unix(1_000_000, 0) }
	t.Cleanup(func() { verifyNow = time.Now; verifyLimit.reset() })
	p := &verifyFakeProber{}
	rt.domainAuto.verify = verifySeams{prober: p, resolver: verifyFakeResolver{hosts: map[string][]string{"far.example.com": {"198.51.100.4"}, "www.far.example.com": {"198.51.100.4"}}}}

	var rep cutoververify.Report
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/verify", &rep); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if rep.Verdict != cutoververify.Fail || p.calls != 0 {
		t.Fatalf("still-elsewhere domain: verdict %s, probe calls %d", rep.Verdict, p.calls)
	}

	verifyLimit.reset()
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

func TestDomainCutoverVerify_PrivateExpectIPRejected(t *testing.T) {
	rt, cookie := cutoverRouter(t, true, new(atomic.Int32))
	verifyLimit.reset()
	p := &verifyFakeProber{}
	rt.domainAuto.verify = verifySeams{prober: p, resolver: verifyFakeResolver{hosts: map[string][]string{"far.example.com": {"127.0.0.1"}}}}
	for _, ip := range []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "172.16.0.9", "fd00::1", "169.254.169.254", "fe80::1", "0.0.0.0", "224.0.0.1", "::1"} {
		if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/verify?expect_ip="+ip, nil); code != http.StatusBadRequest {
			t.Errorf("expect_ip=%s status = %d, want 400", ip, code)
		}
	}
	if p.calls != 0 {
		t.Errorf("prober called %d times", p.calls)
	}
}

// The Host header is attacker controlled; with no configured public address
// the check must fail rather than trust it.
func TestDomainCutoverVerify_HostHeaderIgnored(t *testing.T) {
	rt, cookie := cutoverRouter(t, true, new(atomic.Int32))
	verifyLimit.reset()
	rt.publicHost = ""
	p := &verifyFakeProber{}
	rt.domainAuto.verify = verifySeams{prober: p, resolver: verifyFakeResolver{hosts: map[string][]string{"far.example.com": {"198.51.100.4"}}}}
	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/domains/far.example.com/verify", "")
	req.Host = "198.51.100.4"
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict || p.calls != 0 {
		t.Fatalf("status = %d calls %d body %s: the Host header chose the expected address", w.Code, p.calls, w.Body)
	}
}

func TestDomainCutoverVerify_UnknownServerAddressFails(t *testing.T) {
	rt, cookie := cutoverRouter(t, true, new(atomic.Int32))
	verifyLimit.reset()
	rt.publicHost = ""
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/verify", nil); code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", code)
	}
}

func TestVerifyLimiter(t *testing.T) {
	l := &verifyLimiter{lastActor: map[string]time.Time{}, lastDomain: map[string]time.Time{}}
	t0 := time.Unix(1000, 0)
	rel, _ := l.acquire("alice", "a.example.com", 10*time.Second, t0)
	if rel == nil {
		t.Fatal("first run refused")
	}
	rel()
	if r, wait := l.acquire("alice", "a.example.com", 10*time.Second, t0.Add(3*time.Second)); r != nil || wait <= 0 {
		t.Error("same actor and domain inside the interval must wait")
	}
	if r, _ := l.acquire("bob", "a.example.com", 10*time.Second, t0.Add(time.Second)); r != nil {
		t.Error("another actor inside a quarter interval must wait")
	}
	if r, _ := l.acquire("bob", "a.example.com", 10*time.Second, t0.Add(3*time.Second)); r == nil {
		t.Error("another actor after a quarter interval may run")
	} else {
		r()
	}
	if r, _ := l.acquire("alice", "a.example.com", 10*time.Second, t0.Add(11*time.Second)); r == nil {
		t.Error("run after the interval must pass")
	} else {
		r()
	}
	var held []func()
	for i := range verifyMaxConcurrent {
		r, _ := l.acquire("c", "d"+strconv.Itoa(i)+".example.com", time.Second, t0.Add(time.Hour))
		if r == nil {
			t.Fatalf("run %d refused below the cap", i)
		}
		held = append(held, r)
	}
	if r, _ := l.acquire("c", "over.example.com", time.Second, t0.Add(time.Hour)); r != nil {
		t.Error("run above the concurrency cap must be refused")
	}
	held[0]()
	if r, _ := l.acquire("c", "over.example.com", time.Second, t0.Add(time.Hour)); r == nil {
		t.Error("slot not released")
	}
}
