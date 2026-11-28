package main

import (
	"encoding/json"
	"flag"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/fielddesc"
)

// Every descriptor flag must register on a flag set. Locale keys, docs rows and the flag or
// form only rule for each resource are held by internal/fielddesc's own completeness test.
func TestFieldDescriptorFlagsRegister(t *testing.T) {
	for _, name := range fielddesc.Names() {
		res, _ := fielddesc.Get(name)
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		registerDescriptorFlags(fs, name)
		for _, f := range res.Fields {
			if f.Flag != "" && fs.Lookup(f.Flag) == nil {
				t.Errorf("%s.%s: flag --%s not registered", name, f.ID, f.Flag)
			}
		}
	}
}

func TestFieldsCommandListsEveryResource(t *testing.T) {
	list, _ := runCLIExpectOK(t, []string{"fields"})
	for _, name := range fielddesc.Names() {
		if !strings.Contains(list, name) {
			t.Errorf("fields list misses %s", name)
		}
		table, _ := runCLIExpectOK(t, []string{"fields", name})
		res, _ := fielddesc.Get(name)
		for _, f := range res.Fields {
			if !strings.Contains(table, f.Key) {
				t.Errorf("%s table misses %s", name, f.Key)
			}
		}
		js, _ := runCLIExpectOK(t, []string{"fields", name, "--json"})
		var got fielddesc.Resource
		if err := json.Unmarshal([]byte(js), &got); err != nil || got.Name != name || len(got.Fields) != len(res.Fields) {
			t.Errorf("%s json mismatch: %v", name, err)
		}
	}
}

func TestDNSProviderFlagsArePerProvider(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	flags := registerDescriptorFlagsWhen(fs, fielddesc.DNSProvider, map[string]string{"provider": "route53"})
	if flags["aws-region"] == nil || flags["cf-api-token"] != nil || flags["key-file"] != nil {
		t.Fatalf("route53 flags wrong: %v", flags)
	}
}

func TestDescriptorMultiFlagRepeatsIntoOneList(t *testing.T) {
	for _, name := range []string{fielddesc.EventWebhook, fielddesc.StatusWebhook} {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		flags := registerDescriptorFlags(fs, name)
		if err := fs.Parse([]string{"--event", "a,b", "--event", "c"}); err != nil {
			t.Fatal(err)
		}
		if got := *flags["event"]; got != "a,b,c" {
			t.Errorf("%s: event = %q, want a,b,c", name, got)
		}
	}
}
