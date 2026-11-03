package api

import (
	"bytes"
	"strings"
	"testing"
)

func TestDomainsExportCSVNeutralisesFormulasAndCoversParked(t *testing.T) {
	days := 12
	var buf bytes.Buffer
	err := writeDomainsCSV(&buf,
		[]domainResource{{Domain: "=cmd.example.com", Kind: "app", ServiceName: "web", TLSDaysLeft: &days}},
		[]parkedDomainResource{{Domain: "old.example.com", ParkedFrom: "web", Note: "@sum(1)"}})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"domain,kind,service", "'=cmd.example.com,app,web", "old.example.com,parked", "'@sum(1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("csv missing %q:\n%s", want, out)
		}
	}
}
