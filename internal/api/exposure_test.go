package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/exposure"
)

type exposureLister struct{ states []docker.ContainerState }

func (f exposureLister) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return f.states, nil
}

type fakeIptables struct{ rules []string }

func (f *fakeIptables) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	a := args[1:]
	switch a[0] {
	case "-S":
		out := "-N DOCKER-USER\n"
		for _, r := range f.rules {
			out += "-A DOCKER-USER " + r + "\n"
		}
		return []byte(out), nil
	case "-I":
		f.rules = append([]string{strings.Join(a[3:], " ")}, f.rules...)
	case "-D":
		spec := strings.Join(a[2:], " ")
		for i, r := range f.rules {
			if r == spec {
				f.rules = append(f.rules[:i], f.rules[i+1:]...)
				return nil, nil
			}
		}
		return []byte("no match"), errors.New("exit 1")
	}
	return nil, nil
}

func exposureRouter(t *testing.T, ipt *fakeIptables, states []docker.ContainerState) (*Router, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	mgr := exposure.NewFakeManager("acme:", []int{22, 8080, 9443, 80, 443}, ipt.run,
		func(string) (string, error) { return "/sbin/iptables", nil }, "linux")
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithExposure(mgr, db), WithContainerLister(exposureLister{states}))
	return rt, loginTestSession(t, rt, db)
}

func exposureCall(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec.Code, rec.Body.String()
}

var typesenseState = docker.ContainerState{
	ID: "c1", Name: "typesense", Image: "typesense/typesense:27", Running: true,
	Ports: []docker.PortBinding{{ContainerPort: 8108, HostPort: 8108, HostIP: "0.0.0.0", Protocol: "tcp"}},
}

func TestExposureReportClassifiesPublishedPorts(t *testing.T) {
	ipt := &fakeIptables{rules: []string{"-j RETURN"}}
	loop := docker.ContainerState{ID: "c2", Name: "cache", Image: "redis:7", Running: true,
		Ports: []docker.PortBinding{{ContainerPort: 6379, HostPort: 6379, HostIP: "127.0.0.1", Protocol: "tcp"}}}
	rt, cookie := exposureRouter(t, ipt, []docker.ContainerState{typesenseState, loop})

	code, body := exposureCall(t, rt, cookie, http.MethodGet, "/api/v1/firewall/exposure", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %s", code, body)
	}
	var rep struct {
		Exposed int `json:"exposed"`
		High    int `json:"high"`
		Nodes   []struct {
			Findings []struct {
				Container string `json:"container"`
				Class     string `json:"class"`
				Severity  string `json:"severity"`
				Can       bool   `json:"can_restrict"`
			} `json:"findings"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(body), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Exposed != 1 || rep.High != 1 || len(rep.Nodes) != 1 || len(rep.Nodes[0].Findings) != 2 {
		t.Fatalf("report = %s", body)
	}
	got := map[string]string{}
	for _, f := range rep.Nodes[0].Findings {
		got[f.Container] = f.Class + "/" + f.Severity
	}
	if got["typesense"] != "exposed/high" || got["cache"] != "loopback/info" {
		t.Fatalf("classification = %v", got)
	}
}

func TestExposureRestrictRequiresConfirmAndIsReversible(t *testing.T) {
	ipt := &fakeIptables{rules: []string{"-j RETURN"}}
	rt, cookie := exposureRouter(t, ipt, []docker.ContainerState{typesenseState})
	path := "/api/v1/firewall/exposure/restrictions/tcp/8108"
	body := `{"allow":["10.0.0.2"]}`

	if code, out := exposureCall(t, rt, cookie, http.MethodPut, path, body); code != http.StatusBadRequest || !strings.Contains(out, "confirmation required") {
		t.Fatalf("unconfirmed = %d %s", code, out)
	}
	if len(ipt.rules) != 1 {
		t.Fatalf("rules changed without confirmation: %v", ipt.rules)
	}
	if code, out := exposureCall(t, rt, cookie, http.MethodPut, path, `{"allow":["10.0.0.2"],"confirm":true}`); code != http.StatusOK {
		t.Fatalf("apply = %d %s", code, out)
	}
	if len(ipt.rules) != 3 {
		t.Fatalf("rules = %v", ipt.rules)
	}
	_, out := exposureCall(t, rt, cookie, http.MethodGet, "/api/v1/firewall/exposure", "")
	if !strings.Contains(out, `"class":"restricted"`) || !strings.Contains(out, `"managed":true`) {
		t.Fatalf("not restricted after apply: %s", out)
	}
	if code, out := exposureCall(t, rt, cookie, http.MethodDelete, path, ""); code != http.StatusNoContent {
		t.Fatalf("remove = %d %s", code, out)
	}
	if len(ipt.rules) != 1 || ipt.rules[0] != "-j RETURN" {
		t.Fatalf("rules after remove = %v", ipt.rules)
	}
	rows, err := rt.exposureStore.ListExposureRestrictions(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("stored restrictions = %v, %v", rows, err)
	}
}

func TestExposureRestrictRefusesLockoutAndRemoteNode(t *testing.T) {
	ipt := &fakeIptables{}
	rt, cookie := exposureRouter(t, ipt, nil)
	for _, port := range []string{"22", "80", "443", "8080", "9443"} {
		code, out := exposureCall(t, rt, cookie, http.MethodPut, "/api/v1/firewall/exposure/restrictions/tcp/"+port, `{"allow":["10.0.0.2"],"confirm":true}`)
		if code != http.StatusConflict {
			t.Errorf("port %s = %d %s, want 409", port, code, out)
		}
	}
	code, _ := exposureCall(t, rt, cookie, http.MethodPost, "/api/v1/firewall/exposure/preview", `{"node":"other-node","port":8108,"allow":["10.0.0.2"]}`)
	if code != http.StatusConflict {
		t.Errorf("remote node preview = %d, want 409", code)
	}
	if len(ipt.rules) != 0 {
		t.Fatalf("rules touched: %v", ipt.rules)
	}
}

func TestExposurePreviewChangesNothing(t *testing.T) {
	ipt := &fakeIptables{}
	rt, cookie := exposureRouter(t, ipt, nil)
	code, out := exposureCall(t, rt, cookie, http.MethodPost, "/api/v1/firewall/exposure/preview", `{"port":8108,"allow":["10.0.0.2"],"local_containers":true}`)
	if code != http.StatusOK || !strings.Contains(out, "iptables -w -I DOCKER-USER") || !strings.Contains(out, "172.16.0.0/12") {
		t.Fatalf("preview = %d %s", code, out)
	}
	if len(ipt.rules) != 0 {
		t.Fatalf("preview mutated: %v", ipt.rules)
	}
}

func TestDoctorExposureCheck(t *testing.T) {
	ipt := &fakeIptables{}
	rt, _ := exposureRouter(t, ipt, []docker.ContainerState{typesenseState})
	checks := rt.doctorCheckExposure(context.Background())
	if len(checks) != 2 || checks[0].Code != "exposure" || checks[0].Status != doctorStatusWarn {
		t.Fatalf("checks = %+v", checks)
	}
	if checks[1].Code != "exposure_local_tcp_8108" || !strings.Contains(checks[1].Message, "high") {
		t.Fatalf("finding check = %+v", checks[1])
	}
}
