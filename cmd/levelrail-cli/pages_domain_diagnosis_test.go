package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintPagesDiagnosis(t *testing.T) {
	var b bytes.Buffer
	printPagesDiagnosis(&b, []apiclient.StaticSiteDiagnosis{{Code: "net.port80", OK: false, Hint: "port 80 is closed"}, {Code: "tls.certificate", OK: true, Hint: "issued"}})
	got := b.String()
	if !strings.Contains(got, "[fail] net.port80: port 80 is closed") || !strings.Contains(got, "[ok  ] tls.certificate: issued") {
		t.Fatalf("output = %q", got)
	}
}
