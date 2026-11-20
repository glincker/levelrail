package catalog

import "testing"

func TestNeedsDomainFollowsDomainsAndPorts(t *testing.T) {
	routed := Template{ID: "x", Manifest: &Manifest{Domains: []DomainPrompt{{Service: "a", Port: 80}}}, Compose: "services:\n  a:\n    image: nginx:1\n"}
	if !needsDomain(routed) {
		t.Fatal("a routed template with no published port needs a domain")
	}
	published := routed
	published.Compose = "services:\n  a:\n    image: nginx:1\n    ports:\n      - \"8080:80\"\n"
	if needsDomain(published) {
		t.Fatal("a template publishing a host port works on an IP")
	}
	if needsDomain(Template{ID: "y"}) {
		t.Fatal("a template with no manifest domains does not need one")
	}
}

func TestUpdatedAtRecorded(t *testing.T) {
	if UpdatedAtFor("ackee") == "" {
		t.Fatal("ackee has manifest history and should have a date")
	}
}
