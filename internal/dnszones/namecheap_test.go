package dnszones

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

const ncHostsXML = `<ApiResponse Status="OK"><CommandResponse><DomainDNSGetHostsResult Domain="example.com" IsUsingOurDNS="true">
<host Name="@" Type="A" Address="1.1.1.1" TTL="1800"/>
<host Name="www" Type="CNAME" Address="example.com." TTL="1800"/>
<host Name="@" Type="MX" Address="mail.example.com" MXPref="10" TTL="3600"/>
<host Name="@" Type="TXT" Address="v=spf1 -all" TTL="1800"/>
</DomainDNSGetHostsResult></CommandResponse></ApiResponse>`

func ncServer(t *testing.T, getBody string, set *url.Values) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if strings.Contains(r.URL.RawQuery, "ApiKey") {
			t.Errorf("api key sent in the URL")
		}
		switch r.PostForm.Get("Command") {
		case "namecheap.domains.dns.getHosts":
			_, _ = w.Write([]byte(getBody))
		case "namecheap.domains.dns.setHosts":
			*set = r.PostForm
			_, _ = w.Write([]byte(`<ApiResponse Status="OK"><CommandResponse><DomainDNSSetHostsResult IsSuccess="true"/></CommandResponse></ApiResponse>`))
		}
	}))
}

func TestNamecheapUpsertKeepsExistingRecords(t *testing.T) {
	var set url.Values
	srv := ncServer(t, ncHostsXML, &set)
	defer srv.Close()
	n, err := NewNamecheap("user:key:203.0.113.5")
	if err != nil {
		t.Fatal(err)
	}
	n.SetBaseURL(srv.URL, srv.Client())
	err = n.UpsertRecordSet(context.Background(), Zone{ID: "example.com", Name: "example.com"},
		RecordSet{Name: "app", Type: "A", TTL: 300, Values: []string{"2.2.2.2"}})
	if err != nil {
		t.Fatal(err)
	}
	if set.Get("SLD") != "example" || set.Get("TLD") != "com" {
		t.Fatalf("domain split %v", set)
	}
	seen := map[string]bool{}
	for i := 1; set.Get("HostName"+itoa(i)) != ""; i++ {
		k := set.Get("HostName"+itoa(i)) + "/" + set.Get("RecordType"+itoa(i)) + "/" + set.Get("Address"+itoa(i))
		seen[k] = true
	}
	for _, want := range []string{"@/A/1.1.1.1", "www/CNAME/example.com.", "@/MX/mail.example.com", "@/TXT/v=spf1 -all", "app/A/2.2.2.2"} {
		if !seen[want] {
			t.Errorf("record %s missing from the write, got %v", want, seen)
		}
	}
	if len(seen) != 5 {
		t.Errorf("want 5 records, got %d", len(seen))
	}
}

func TestNamecheapRefusesEmptyOrFailedRead(t *testing.T) {
	for name, body := range map[string]string{
		"empty": `<ApiResponse Status="OK"><CommandResponse><DomainDNSGetHostsResult Domain="example.com" IsUsingOurDNS="true"/></CommandResponse></ApiResponse>`,
		"error": `<ApiResponse Status="ERROR"><Errors><Error Number="1">Invalid IP</Error></Errors></ApiResponse>`,
	} {
		var set url.Values
		srv := ncServer(t, body, &set)
		n, _ := NewNamecheap("user:key:203.0.113.5")
		n.SetBaseURL(srv.URL, srv.Client())
		err := n.UpsertRecordSet(context.Background(), Zone{Name: "example.com"}, RecordSet{Name: "a", Type: "A", Values: []string{"1.1.1.1"}})
		srv.Close()
		if err == nil || set != nil {
			t.Errorf("%s: err %v, write %v", name, err, set)
		}
	}
}

func TestNamecheapCredentialShape(t *testing.T) {
	if _, err := NewNamecheap("user:key"); err == nil {
		t.Fatal("two part credential accepted")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
