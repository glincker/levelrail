package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/exposure"
)

func exposureTestFinding(container string, owner exposure.OwnerKind, port int, proto string, sev exposure.Severity) exposureFindingResource {
	return exposureFindingResource{Finding: exposure.Finding{
		Container: container, Owner: exposure.Owner{Kind: owner}, HostPort: port, Protocol: proto,
		Class: exposure.ClassExposed, Severity: sev, CanRestrict: true,
	}}
}

func TestDoctorExposureForeignProxyGroupedAdvisory(t *testing.T) {
	n := exposureNodeResource{NodeName: "srv1", Local: true, Status: exposureNodeOK}
	for _, p := range []struct {
		port  int
		proto string
	}{{80, "tcp"}, {443, "tcp"}, {443, "udp"}, {8080, "tcp"}} {
		n.Findings = append(n.Findings, exposureTestFinding("coolify-proxy", exposure.OwnerUnmanaged, p.port, p.proto, exposure.SeverityLow))
	}
	var tally exposureTally
	rows := doctorExposureContainerRows(n, &tally)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.Status != doctorStatusOK {
		t.Fatalf("status = %s, want ok", r.Status)
	}
	for _, want := range []string{"80/tcp", "443/tcp", "443/udp", "8080/tcp", "managed by another tool"} {
		if !strings.Contains(r.Message, want) {
			t.Fatalf("message %q lacks %q", r.Message, want)
		}
	}
	if strings.Count(r.Message, "Public ports") != 1 {
		t.Fatalf("duplicated text: %q", r.Message)
	}
	if r.Fix == "" || r.DocsPath == "" {
		t.Fatal("row has no call to action")
	}
	sum := doctorExposureSummary(tally)
	if sum.Status != doctorStatusOK || !strings.Contains(sum.Message, "4 published") || !strings.Contains(sum.Message, "1 container(s) managed by another tool") {
		t.Fatalf("summary: %s %q", sum.Status, sum.Message)
	}
}

func TestDoctorExposureManagedDatabasePortStaysWarning(t *testing.T) {
	n := exposureNodeResource{NodeName: "srv1", Local: true, Status: exposureNodeOK, Findings: []exposureFindingResource{
		exposureTestFinding("pg-main", exposure.OwnerDatabase, 5432, "tcp", exposure.SeverityHigh),
		exposureTestFinding("proxy", exposure.OwnerUnmanaged, 80, "tcp", exposure.SeverityLow),
	}}
	var tally exposureTally
	rows := doctorExposureContainerRows(n, &tally)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	byName := map[string]doctorCheckResource{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	if byName["Container pg-main on srv1"].Status != doctorStatusWarn || byName["Container proxy on srv1"].Status != doctorStatusOK {
		t.Fatalf("statuses wrong: %+v", rows)
	}
	sum := doctorExposureSummary(tally)
	if sum.Status != doctorStatusWarn || !strings.Contains(sum.Message, "2 published container port(s) in 2 container(s)") || !strings.Contains(sum.Message, "1 container(s) managed by another tool") {
		t.Fatalf("summary: %s %q", sum.Status, sum.Message)
	}
}
