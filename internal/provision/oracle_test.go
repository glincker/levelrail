package provision

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const (
	tTenancy  = "ocid1.tenancy.oc1..aaaaaaaa"
	tUser     = "ocid1.user.oc1..bbbbbbbb"
	tComp     = "ocid1.compartment.oc1..cccccccc"
	tSubnet   = "ocid1.subnet.oc1.phx.dddddddd"
	tInstance = "ocid1.instance.oc1.phx.eeeeeeee"
	tFP       = "aa:bb:cc"
)

var tAuthRe = regexp.MustCompile(`keyId="([^"]+)",algorithm="rsa-sha256",headers="([^"]+)",signature="([^"]+)"`)

type fakeOCI struct {
	mu        sync.Mutex
	state     string
	launchAD  []string
	capacity  map[string]bool
	launchErr int
	launchMsg string
	deleted   string
	delStatus int
	delQuery  string
	last      map[string]any
	srv       *httptest.Server
}

func newFakeOCI(t *testing.T, pub *rsa.PublicKey) *fakeOCI {
	f := &fakeOCI{state: "RUNNING", capacity: map[string]bool{}, delStatus: 204}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !verified(r, pub) {
			http.Error(w, `{"code":"NotAuthenticated","message":"secret-body-ocid"}`, http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/regionSubscriptions"):
			_, _ = w.Write([]byte(`[{"regionName":"us-phoenix-1","status":"READY"},{"regionName":"us-ashburn-1","status":"IN_PROGRESS"}]`))
		case strings.HasSuffix(p, "/availabilityDomains"):
			_, _ = w.Write([]byte(`[{"name":"x:PHX-AD-1"},{"name":"x:PHX-AD-2"}]`))
		case strings.HasSuffix(p, "/shapes"):
			_, _ = w.Write([]byte(`[{"shape":"VM.Standard.E2.1.Micro","ocpus":1,"memoryInGBs":1},{"shape":"VM.Standard.A1.Flex","ocpuOptions":{"min":1,"max":80},"memoryOptions":{"minInGBs":1,"maxInGBs":512}}]`))
		case strings.HasSuffix(p, "/images"):
			_, _ = w.Write([]byte(`[{"id":"ocid1.image.oc1.phx.ffffffff"}]`))
		case p == "/instances" && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var d map[string]any
			_ = json.Unmarshal(body, &d)
			f.last = d
			ad, _ := d["availabilityDomain"].(string)
			f.launchAD = append(f.launchAD, ad)
			if f.capacity[ad] {
				w.WriteHeader(500)
				_, _ = w.Write([]byte(`{"code":"InternalError","message":"Out of host capacity for shape ocid1.tenancy.secret"}`))
				return
			}
			if f.launchErr != 0 {
				w.WriteHeader(f.launchErr)
				_, _ = w.Write([]byte(`{"code":"LimitExceeded","message":"` + f.launchMsg + `"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"` + tInstance + `"}`))
		case strings.HasPrefix(p, "/instances/") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"lifecycleState":"` + f.state + `"}`))
		case strings.HasPrefix(p, "/instances/") && r.Method == http.MethodDelete:
			f.deleted, f.delQuery = p, r.URL.RawQuery
			w.WriteHeader(f.delStatus)
			if f.delStatus != 204 {
				_, _ = w.Write([]byte(`{"code":"NotAuthorizedOrNotFound","message":"nope"}`))
			}
		case strings.HasSuffix(p, "/vnicAttachments"):
			_, _ = w.Write([]byte(`[{"vnicId":"ocid1.vnic.oc1.phx.gggggggg","lifecycleState":"ATTACHED"}]`))
		case strings.HasPrefix(p, "/vnics/"):
			_, _ = w.Write([]byte(`{"publicIp":"203.0.113.9","isPrimary":true}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// verified rejects anything whose signature does not check out, like OCI does.
func verified(r *http.Request, pub *rsa.PublicKey) bool {
	m := tAuthRe.FindStringSubmatch(r.Header.Get("Authorization"))
	if m == nil || m[1] != tTenancy+"/"+tUser+"/"+tFP {
		return false
	}
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if r.Method == http.MethodPost {
		sum := sha256.Sum256(body)
		if r.Header.Get("X-Content-Sha256") != base64.StdEncoding.EncodeToString(sum[:]) {
			return false
		}
	}
	want := "date (request-target) host"
	if r.Method == http.MethodPost {
		want += " content-length content-type x-content-sha256"
	}
	if m[2] != want {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(m[3])
	if err != nil {
		return false
	}
	lines := []string{"date: " + r.Header.Get("Date"), "(request-target): " + strings.ToLower(r.Method) + " " + r.URL.RequestURI(), "host: " + r.Host}
	if r.Method == http.MethodPost {
		lines = append(lines, "content-length: "+r.Header.Get("Content-Length"), "content-type: "+r.Header.Get("Content-Type"), "x-content-sha256: "+r.Header.Get("X-Content-Sha256"))
	}
	d := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, d[:], sig) == nil
}

func newTestOracle(t *testing.T) (*Oracle, *fakeOCI) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	f := newFakeOCI(t, &key.PublicKey)
	o, err := newOracle(OracleCredential{
		TenancyOCID: tTenancy, UserOCID: tUser, Fingerprint: tFP, PrivateKeyPEM: pemText,
		CompartmentOCID: tComp, SubnetOCID: tSubnet, Region: "us-phoenix-1",
	}, OracleSettings{}, f.srv.URL, f.srv.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	return o, f
}

func TestOracleRegionsAndSizes(t *testing.T) {
	o, _ := newTestOracle(t)
	regs, err := o.ListRegions(context.Background())
	if err != nil || len(regs) != 1 || regs[0].ID != "us-phoenix-1" {
		t.Fatalf("regions %v %v", regs, err)
	}
	sizes, err := o.ListSizes(context.Background(), "us-phoenix-1")
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, s := range sizes {
		ids = append(ids, s.ID)
		if s.PriceMonthly != "" {
			t.Fatal("oracle sizes must be unpriced")
		}
	}
	if strings.Join(ids, ",") != "VM.Standard.A1.Flex:1:6,VM.Standard.A1.Flex:2:12,VM.Standard.E2.1.Micro" {
		t.Fatalf("ids %v", ids)
	}
}

func TestOracleUnverifiableSignatureRejected(t *testing.T) {
	o, _ := newTestOracle(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	o.signer.Key = other
	_, err := o.ListRegions(context.Background())
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != 401 {
		t.Fatalf("want 401, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-body-ocid") || strings.Contains(err.Error(), "NotAuthenticated") {
		t.Fatalf("error leaked response body: %v", err)
	}
}

func TestOracleCreateStatusDestroy(t *testing.T) {
	o, f := newTestOracle(t)
	ctx := context.Background()
	id, ip, err := o.CreateServer(ctx, CreateOpts{Region: "us-phoenix-1", Size: "VM.Standard.A1.Flex:1:6", Name: "n1", UserData: "#cloud-config"})
	if err != nil || id != tInstance || ip != "" {
		t.Fatalf("create %q %q %v", id, ip, err)
	}
	cfg := f.last["shapeConfig"].(map[string]any)
	if cfg["ocpus"].(float64) != 1 || cfg["memoryInGBs"].(float64) != 6 {
		t.Fatalf("shapeConfig %v", cfg)
	}
	md := f.last["metadata"].(map[string]any)["user_data"].(string)
	if dec, _ := base64.StdEncoding.DecodeString(md); string(dec) != "#cloud-config" {
		t.Fatal("user_data not base64 cloud-init")
	}
	st, ip, err := o.GetServer(ctx, id)
	if err != nil || st != ServerStatusRunning || ip != "203.0.113.9" {
		t.Fatalf("get %v %q %v", st, ip, err)
	}
	f.state = "PROVISIONING"
	if st, _, _ = o.GetServer(ctx, id); st != ServerStatusPending {
		t.Fatalf("status %v", st)
	}
	f.state = "TERMINATED"
	if st, _, _ = o.GetServer(ctx, id); st != ServerStatusError {
		t.Fatalf("status %v", st)
	}
	if err := o.DeleteServer(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.delQuery, "preserveBootVolume=false") {
		t.Fatalf("delete query %q", f.delQuery)
	}
}

func TestOracleCapacityTriesNextAD(t *testing.T) {
	o, f := newTestOracle(t)
	f.capacity["x:PHX-AD-1"] = true
	if _, _, err := o.CreateServer(context.Background(), CreateOpts{Size: "VM.Standard.E2.1.Micro", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if len(f.launchAD) != 2 || f.launchAD[1] != "x:PHX-AD-2" {
		t.Fatalf("ads %v", f.launchAD)
	}
	f.capacity["x:PHX-AD-2"] = true
	_, _, err := o.CreateServer(context.Background(), CreateOpts{Size: "VM.Standard.E2.1.Micro", Name: "n"})
	if !errors.Is(err, errOracleNoCapacity) {
		t.Fatalf("want no capacity, got %v", err)
	}
	if strings.Contains(err.Error(), "ocid1") {
		t.Fatal("capacity error leaked body")
	}
}

func TestOracleLimitExceededStops(t *testing.T) {
	o, f := newTestOracle(t)
	f.launchErr, f.launchMsg = 400, "quota ocid1.tenancy.secret"
	_, _, err := o.CreateServer(context.Background(), CreateOpts{Size: "VM.Standard.E2.1.Micro", Name: "n"})
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != 400 || len(f.launchAD) != 1 {
		t.Fatalf("err %v tries %v", err, f.launchAD)
	}
	if strings.Contains(err.Error(), "ocid1") || strings.Contains(err.Error(), "quota") {
		t.Fatalf("leaked body: %v", err)
	}
}

func TestOracleDelete404IsSuccess(t *testing.T) {
	o, f := newTestOracle(t)
	f.delStatus = 404
	if err := o.DeleteServer(context.Background(), tInstance); err != nil {
		t.Fatal(err)
	}
	f.delStatus = 500
	err := o.DeleteServer(context.Background(), tInstance)
	if err == nil || strings.Contains(err.Error(), "NotAuthorized") {
		t.Fatalf("err %v", err)
	}
}

func TestOracleRejectsBadSizeAndKeyErrors(t *testing.T) {
	o, _ := newTestOracle(t)
	if _, _, err := o.CreateServer(context.Background(), CreateOpts{Size: "VM.Standard.A1.Flex:99999:6", Name: "n"}); err == nil {
		t.Fatal("out of range flex size accepted")
	}
	_, err := newOracle(OracleCredential{TenancyOCID: tTenancy, UserOCID: tUser, Fingerprint: tFP,
		PrivateKeyPEM: "-----BEGIN PRIVATE KEY-----\nTOPSECRETKEY\n-----END PRIVATE KEY-----", CompartmentOCID: tComp, Region: "us-phoenix-1"}, OracleSettings{}, "", "", false)
	if err == nil || strings.Contains(err.Error(), "TOPSECRETKEY") {
		t.Fatalf("err %v", err)
	}
}
