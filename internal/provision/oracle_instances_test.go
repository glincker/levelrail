package provision

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type ociCall struct {
	method, path, query, body string
}

type instFakeOCI struct {
	mu     sync.Mutex
	calls  []ociCall
	doc    string
	status map[string]int
	// att is the vnicAttachments body; attStatus forces its status; vnics maps vnic id to body.
	att       string
	attStatus int
	vnics     map[string]string
}

func (f *instFakeOCI) find(method string) (ociCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.method == method {
			return c, true
		}
	}
	return ociCall{}, false
}

func newInstOracle(t *testing.T, doc string) (*Oracle, *instFakeOCI) {
	t.Helper()
	f := &instFakeOCI{doc: doc, status: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, ociCall{r.Method, r.URL.Path, r.URL.RawQuery, string(b)})
		st := f.status[r.Method]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if st != 0 {
			w.WriteHeader(st)
			_, _ = w.Write([]byte(`{"code":"IncorrectState","message":"secret"}`))
			return
		}
		switch {
		case r.URL.Path == "/vnicAttachments" && f.attStatus != 0:
			w.WriteHeader(f.attStatus)
			_, _ = w.Write([]byte(`{"code":"NotAuthorizedOrNotFound"}`))
		case r.URL.Path == "/vnicAttachments" && f.att != "":
			_, _ = w.Write([]byte(f.att))
		case strings.HasPrefix(r.URL.Path, "/vnics/") && f.vnics[strings.TrimPrefix(r.URL.Path, "/vnics/")] != "":
			_, _ = w.Write([]byte(f.vnics[strings.TrimPrefix(r.URL.Path, "/vnics/")]))
		case r.URL.Path == "/instances" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[` + f.doc + `,{"id":"` + tInstance + `x","lifecycleState":"TERMINATED"}]`))
		case strings.HasPrefix(r.URL.Path, "/instances/") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(f.doc))
		case strings.HasPrefix(r.URL.Path, "/instances/"):
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/auditEvents":
			_, _ = w.Write([]byte(`[{"eventName":"InstanceAction","eventTime":"2026-11-20T10:00:00Z","data":{"resourceId":"` + tInstance + `","response":{"status":"200"}}},{"eventName":"Other","eventTime":"2026-11-20T11:00:00Z","data":{"resourceId":"ocid1.instance.oc1.phx.other"}}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	o, err := newOracle(OracleCredential{
		TenancyOCID: tTenancy, UserOCID: tUser, Fingerprint: tFP, PrivateKeyPEM: pemText,
		CompartmentOCID: tComp, Region: "us-phoenix-1",
	}, OracleSettings{}, srv.URL, srv.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	return o, f
}

const flexDoc = `{"id":"` + tInstance + `","displayName":"web","lifecycleState":"RUNNING","shape":"VM.Standard.A1.Flex",` +
	`"availabilityDomain":"x:PHX-AD-1","freeformTags":{"levelrail-provision-id":"p1","team":"a"},` +
	`"shapeConfig":{"ocpus":1,"memoryInGBs":6},"sourceDetails":{"imageId":"ocid1.image.oc1.phx.f","bootVolumeSizeInGBs":50}}`

func TestOracleInstanceActionsMapQuery(t *testing.T) {
	want := map[InstanceActionKind]string{
		InstanceActionStart: "START", InstanceActionStop: "STOP", InstanceActionSoftStop: "SOFTSTOP",
		InstanceActionReboot: "RESET", InstanceActionSoftReboot: "SOFTRESET",
	}
	for kind, action := range want {
		o, f := newInstOracle(t, flexDoc)
		if err := o.InstanceAction(context.Background(), tInstance, kind); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		c, ok := f.find("POST")
		if !ok || c.query != "action="+action || c.path != "/instances/"+tInstance {
			t.Fatalf("%s: call %+v", kind, c)
		}
	}
	o, _ := newInstOracle(t, flexDoc)
	if err := o.InstanceAction(context.Background(), tInstance, "bogus"); !errors.Is(err, ErrInstanceUnsupported) {
		t.Fatalf("bogus action: %v", err)
	}
}

func TestOracleResizeBodyAndFixedShape(t *testing.T) {
	o, f := newInstOracle(t, flexDoc)
	if err := o.ResizeInstance(context.Background(), tInstance, ResizeSpec{OCPUs: 2, MemoryGB: 12}); err != nil {
		t.Fatal(err)
	}
	c, _ := f.find("PUT")
	var body map[string]map[string]float64
	if err := json.Unmarshal([]byte(c.body), &body); err != nil || body["shapeConfig"]["ocpus"] != 2 || body["shapeConfig"]["memoryInGBs"] != 12 {
		t.Fatalf("body %q %v", c.body, err)
	}
	fixed := `{"id":"` + tInstance + `","lifecycleState":"RUNNING","shape":"VM.Standard.E2.1.Micro"}`
	o2, f2 := newInstOracle(t, fixed)
	if err := o2.ResizeInstance(context.Background(), tInstance, ResizeSpec{OCPUs: 2, MemoryGB: 12}); !errors.Is(err, ErrInstanceUnsupported) {
		t.Fatalf("fixed shape: %v", err)
	}
	if _, ok := f2.find("PUT"); ok {
		t.Fatal("fixed shape must not send a PUT")
	}
}

func TestOracleTerminatePreserveBootVolume(t *testing.T) {
	for _, keep := range []bool{true, false} {
		o, f := newInstOracle(t, flexDoc)
		if err := o.TerminateInstance(context.Background(), tInstance, keep); err != nil {
			t.Fatal(err)
		}
		c, _ := f.find("DELETE")
		want := "preserveBootVolume=false"
		if keep {
			want = "preserveBootVolume=true"
		}
		if c.query != want {
			t.Fatalf("keep=%v query %q", keep, c.query)
		}
	}
}

func TestOracleTagsMergeKeepsLabel(t *testing.T) {
	o, f := newInstOracle(t, flexDoc)
	if err := o.SetInstanceTags(context.Background(), tInstance, map[string]string{"env": "prod", "team": ""}); err != nil {
		t.Fatal(err)
	}
	c, _ := f.find("PUT")
	var body struct {
		Tags map[string]string `json:"freeformTags"`
	}
	_ = json.Unmarshal([]byte(c.body), &body)
	if body.Tags["levelrail-provision-id"] != "p1" || body.Tags["env"] != "prod" || len(body.Tags) != 2 {
		t.Fatalf("tags %v", body.Tags)
	}
	if err := o.SetInstanceTags(context.Background(), tInstance, map[string]string{"levelrail-provision-id": "x"}); !errors.Is(err, ErrInstanceInvalid) {
		t.Fatalf("label overwrite: %v", err)
	}
}

func TestOracleListDescribeEventsAndErrors(t *testing.T) {
	o, f := newInstOracle(t, flexDoc)
	rows, err := o.ListInstances(context.Background(), InstanceScope{})
	if err != nil || len(rows) != 1 || !rows[0].Managed || rows[0].State != InstanceStateRunning || rows[0].OCPUs != 1 {
		t.Fatalf("list %+v %v", rows, err)
	}
	d, err := o.DescribeInstance(context.Background(), tInstance)
	if err != nil || d.BootVolumeGB != 50 || d.Zone != "x:PHX-AD-1" || d.ResizeAdvisory == "" {
		t.Fatalf("describe %+v %v", d, err)
	}
	ev, err := o.InstanceEvents(context.Background(), tInstance, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(ev) != 1 || ev[0].Kind != "InstanceAction" || ev[0].Status != "200" {
		t.Fatalf("events %+v %v", ev, err)
	}
	f.mu.Lock()
	f.status["POST"] = 409
	f.mu.Unlock()
	err = o.InstanceAction(context.Background(), tInstance, InstanceActionStop)
	if !errors.Is(err, ErrInstanceConflict) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("conflict: %v", err)
	}
	if !InstanceCapabilities(o).SupportsAction(InstanceActionReboot) {
		t.Fatal("oracle should support reboot")
	}
}
