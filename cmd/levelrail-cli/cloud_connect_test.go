package main

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // OCI fingerprint format
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
)

const (
	tTenancy = "ocid1.tenancy.oc1..aaaaaaaatenancy1"
	tComp    = "ocid1.compartment.oc1..aaaaaaaacomp0001"
	tGroup   = "ocid1.group.oc1..aaaaaaaagroup001"
	tUser    = "ocid1.user.oc1..aaaaaaaauser0001"
	tPolicy  = "ocid1.policy.oc1..aaaaaaaapolicy01"
)

var (
	testKeyOnce sync.Once
	testKey     *rsa.PrivateKey
)

func connectTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testKeyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		testKey = k
	})
	return testKey
}

func fpOf(t *testing.T, pub *rsa.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(der) //nolint:gosec // OCI fingerprint format
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, ":")
}

type fakeOCIRunner struct {
	t      *testing.T
	calls  [][]string
	failOn string
	pubPEM []byte
}

func (f *fakeOCIRunner) Run(_ context.Context, _ string, args []string) ([]byte, error) {
	f.calls = append(f.calls, args)
	joined := strings.Join(args, " ")
	if f.failOn != "" && strings.HasPrefix(joined, f.failOn) {
		return nil, errors.New("ServiceError: NotAuthorizedOrNotFound token eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0.c2lnbmF0dXJl")
	}
	switch {
	case strings.HasPrefix(joined, "iam availability-domain list"):
		return []byte(`{"data":[{"compartment-id":"` + tTenancy + `"}]}`), nil
	case strings.HasPrefix(joined, "iam compartment create"):
		return []byte(`{"data":{"compartment-id":"` + tTenancy + `","id":"` + tComp + `"}}`), nil
	case strings.HasPrefix(joined, "iam group create"):
		return []byte(`{"data":{"id":"` + tGroup + `"}}`), nil
	case strings.HasPrefix(joined, "iam user create"):
		return []byte(`{"data":{"id":"` + tUser + `"}}`), nil
	case strings.HasPrefix(joined, "iam policy create"):
		return []byte(`{"data":{"id":"` + tPolicy + `"}}`), nil
	case strings.HasPrefix(joined, "iam user api-key upload"):
		path := args[len(args)-1]
		for i, a := range args {
			if a == "--key-file" {
				path = args[i+1]
			}
		}
		raw, err := os.ReadFile(path) //nolint:gosec // test temp file
		if err != nil {
			return nil, err
		}
		f.pubPEM = raw
		blk, _ := pem.Decode(raw)
		pub, err := x509.ParsePKIXPublicKey(blk.Bytes)
		if err != nil {
			return nil, err
		}
		return []byte(`{"data":{"fingerprint":"` + fpOf(f.t, pub.(*rsa.PublicKey)) + `"}}`), nil
	}
	return []byte(`{"data":[]}`), nil
}

type fakeOCISink struct {
	notReady error
	stored   []cloudconn.OracleCredential
}

func (s *fakeOCISink) Ready(context.Context) error { return s.notReady }
func (s *fakeOCISink) Store(_ context.Context, _ string, c cloudconn.OracleCredential) (string, error) {
	s.stored = append(s.stored, c)
	return "aa:bb:cc", nil
}

func ociTestDeps(t *testing.T, r *fakeOCIRunner, sink *fakeOCISink, env map[string]string) ociConnectDeps {
	key := connectTestKey(t)
	return ociConnectDeps{
		runner:   r,
		lookPath: func(string) (string, error) { return "/usr/bin/oci", nil },
		sink:     func(string, string) ociCredentialSink { return sink },
		stdin:    strings.NewReader(""),
		tempBase: t.TempDir(),
		genKey:   func() (*rsa.PrivateKey, error) { return key, nil },
		lookup:   func(k string) (string, bool) { v, ok := env[k]; return v, ok },
	}
}

func privPEMOf(k *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
}

func keyBody(k *rsa.PrivateKey) string {
	// A middle line of the base64 body: any leak of the key would contain it.
	lines := strings.Split(privPEMOf(k), "\n")
	return lines[5]
}

func runConnect(_ *testing.T, d ociConnectDeps, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runCloudConnectOCI("cli", args, &out, &errb, d.lookup, d)
	return code, out.String(), errb.String()
}

func TestCloudConnectOCIDryRunRunsNothing(t *testing.T) {
	r := &fakeOCIRunner{t: t}
	sink := &fakeOCISink{}
	code, out, errOut := runConnect(t, ociTestDeps(t, r, sink, nil), "--region", "us-phoenix-1")
	if code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	for _, c := range r.calls {
		j := strings.Join(c, " ")
		if strings.Contains(j, "create") || strings.Contains(j, "upload") || strings.Contains(j, "add-user") {
			t.Fatalf("dry run ran a mutating command: %s", j)
		}
	}
	for _, want := range []string{"1. create the compartment levelrail", "iam policy create", "7. upload", "Dry run: nothing was created", "in compartment id <compartment-ocid>", "--auth security_token"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan missing %q:\n%s", want, out)
		}
	}
	if len(sink.stored) != 0 {
		t.Fatal("dry run stored a credential")
	}
}

func TestCloudConnectOCIYesRunsInOrder(t *testing.T) {
	r := &fakeOCIRunner{t: t}
	sink := &fakeOCISink{}
	d := ociTestDeps(t, r, sink, nil)
	code, out, errOut := runConnect(t, d, "--region", "us-phoenix-1", "--yes", "--name", "lab")
	if code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	var verbs []string
	for _, c := range r.calls {
		verbs = append(verbs, strings.Join(c[:3], " "))
		for _, a := range c {
			if strings.Contains(a, "PRIVATE KEY") {
				t.Fatal("private key reached argv")
			}
		}
		if !strings.Contains(strings.Join(c, " "), "--auth security_token --profile DEFAULT --region us-phoenix-1") {
			t.Errorf("call without session auth: %v", c)
		}
	}
	want := []string{"iam region list", "iam availability-domain list", "iam compartment create", "iam group create", "iam user create", "iam user update-user-capabilities", "iam group add-user", "iam policy create", "iam user api-key"}
	if strings.Join(verbs, "|") != strings.Join(want, "|") {
		t.Fatalf("order:\n got %v\nwant %v", verbs, want)
	}
	if strings.Contains(string(r.pubPEM), "PRIVATE") {
		t.Fatal("uploaded file holds a private key")
	}
	if len(sink.stored) != 1 {
		t.Fatal("credential not stored")
	}
	c := sink.stored[0]
	if c.TenancyOCID != tTenancy || c.CompartmentOCID != tComp || c.Secret.UserOCID != tUser || c.Region != "us-phoenix-1" {
		t.Fatalf("stored %v", c)
	}
	body := keyBody(connectTestKey(t))
	if strings.Contains(out+errOut, body) || strings.Contains(out+errOut, "PRIVATE KEY") {
		t.Fatal("private key printed")
	}
	if ents, _ := os.ReadDir(d.tempBase); len(ents) != 0 {
		t.Fatalf("temp key dir left behind: %v", ents)
	}
}

func TestCloudConnectOCIFailureListsCleanup(t *testing.T) {
	r := &fakeOCIRunner{t: t, failOn: "iam policy create"}
	sink := &fakeOCISink{}
	d := ociTestDeps(t, r, sink, nil)
	code, out, errOut := runConnect(t, d, "--region", "us-phoenix-1", "--tenancy", tTenancy, "--yes")
	if code == exitOK {
		t.Fatal("expected failure")
	}
	for _, want := range []string{"group remove-user --group-id " + tGroup, "user delete --user-id " + tUser, "group delete --group-id " + tGroup, "compartment delete --compartment-id " + tComp} {
		if !strings.Contains(errOut, want) {
			t.Errorf("cleanup missing %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "policy delete") || strings.Contains(errOut, "eyJhbGci") {
		t.Fatalf("unexpected cleanup or token leak:\n%s", errOut)
	}
	for _, c := range r.calls {
		if strings.Contains(strings.Join(c, " "), "delete") {
			t.Fatal("cleanup must never run on its own")
		}
	}
	if len(sink.stored) != 0 || strings.Contains(out+errOut, keyBody(connectTestKey(t))) {
		t.Fatal("stored or leaked after failure")
	}
	if ents, _ := os.ReadDir(d.tempBase); len(ents) != 0 {
		t.Fatal("temp key dir left behind")
	}
}

func TestCloudConnectOCIExpiredSession(t *testing.T) {
	r := &fakeOCIRunner{t: t, failOn: "iam region list"}
	code, _, errOut := runConnect(t, ociTestDeps(t, r, &fakeOCISink{}, nil), "--yes")
	if code == exitOK || !strings.Contains(errOut, "oci session authenticate") || strings.Contains(errOut, "eyJhbGci") {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if len(r.calls) != 1 {
		t.Fatalf("ran past the session check: %v", r.calls)
	}
}

func TestCloudConnectOCISinkNotReadyCreatesNothing(t *testing.T) {
	r := &fakeOCIRunner{t: t}
	code, _, errOut := runConnect(t, ociTestDeps(t, r, &fakeOCISink{notReady: errors.New("api not ready")}, nil), "--region", "us-phoenix-1", "--yes")
	if code == exitOK || !strings.Contains(errOut, "Nothing was created") {
		t.Fatalf("code %d: %s", code, errOut)
	}
	for _, c := range r.calls {
		if strings.Contains(strings.Join(c, " "), "create") {
			t.Fatal("created resources while the API could not take the key")
		}
	}
	if secureTransport("http://levelrail.example.com") == nil || secureTransport("http://127.0.0.1:8080") != nil || secureTransport("https://levelrail.example.com") != nil {
		t.Fatal("transport rule wrong")
	}
}

func TestCloudConnectOCIPolicyNeverTenancyWide(t *testing.T) {
	o := ociConnectOpts{compartmentName: "levelrail", name: "oci"}
	st, err := ociPolicy(o, tComp)
	if err != nil || len(st) == 0 {
		t.Fatal(err)
	}
	for _, s := range st {
		if !strings.Contains(s, "in compartment id "+tComp) || strings.Contains(s, "tenancy") {
			t.Fatalf("not compartment scoped: %s", s)
		}
	}
	for _, bad := range []string{"tenancy", "root"} {
		if _, err := ociPolicy(ociConnectOpts{compartmentName: bad, name: "oci"}, ""); err == nil {
			t.Fatalf("compartment name %q accepted", bad)
		}
	}
	if _, err := ociPolicy(o, tTenancy); err == nil {
		t.Fatal("tenancy OCID accepted as policy scope")
	}
}

func TestCloudConnectOCIPaste(t *testing.T) {
	k := connectTestKey(t)
	env := map[string]string{"OCI_KEY": privPEMOf(k)}
	base := []string{"--paste", "--tenancy", tTenancy, "--region", "us-phoenix-1", "--user-ocid", tUser, "--fingerprint", fpOf(t, &k.PublicKey), "--key-env", "OCI_KEY"}

	sink := &fakeOCISink{}
	r := &fakeOCIRunner{t: t}
	code, out, errOut := runConnect(t, ociTestDeps(t, r, sink, env), append(base, "--compartment-ocid", tComp, "--yes")...)
	if code != exitOK || len(sink.stored) != 1 || len(r.calls) != 0 {
		t.Fatalf("code %d stored %d calls %d: %s", code, len(sink.stored), len(r.calls), errOut)
	}
	if strings.Contains(out+errOut, keyBody(k)) {
		t.Fatal("key printed")
	}

	sink = &fakeOCISink{}
	code, _, errOut = runConnect(t, ociTestDeps(t, r, sink, env), append(base, "--compartment-ocid", tTenancy, "--yes")...)
	if code == exitOK || len(sink.stored) != 0 || !strings.Contains(errOut, "tenancy root is never accepted") {
		t.Fatalf("tenancy root accepted: %d %s", code, errOut)
	}

	bad := append([]string{}, base...)
	bad[8] = "aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99"
	code, _, errOut = runConnect(t, ociTestDeps(t, r, sink, env), append(bad, "--compartment-ocid", tComp, "--yes")...)
	if code == exitOK || len(sink.stored) != 0 || strings.Contains(errOut, keyBody(k)) {
		t.Fatalf("mismatched fingerprint accepted: %d %s", code, errOut)
	}
}

func TestCloudConnectOCIJSON(t *testing.T) {
	r := &fakeOCIRunner{t: t}
	sink := &fakeOCISink{}
	code, out, errOut := runConnect(t, ociTestDeps(t, r, sink, nil), "--region", "us-phoenix-1", "--json")
	var plan ociResult
	if code != exitOK || json.Unmarshal([]byte(out), &plan) != nil || !plan.DryRun || len(plan.Steps) < 2 || len(sink.stored) != 0 {
		t.Fatalf("dry run json: %d %s %s", code, out, errOut)
	}

	k := connectTestKey(t)
	code, out, errOut = runConnect(t, ociTestDeps(t, &fakeOCIRunner{t: t}, sink, nil), "--region", "us-phoenix-1", "--json", "--yes")
	var res ociResult
	if code != exitOK || json.Unmarshal([]byte(out), &res) != nil || res.DryRun || res.Connection != "oci" || res.FingerprintPrefix != "aa:bb:cc" || res.CompartmentID == "" {
		t.Fatalf("yes json: %d %s %s", code, out, errOut)
	}
	if strings.Contains(out+errOut, keyBody(k)) || strings.Contains(out, "PRIVATE KEY") {
		t.Fatal("key printed in json")
	}
}

func TestCloudConnectOCIJSONRefusesPromptAndReportsErrors(t *testing.T) {
	k := connectTestKey(t)
	sink := &fakeOCISink{}
	deps := ociTestDeps(t, &fakeOCIRunner{t: t}, sink, nil)
	code, out, _ := runConnect(t, deps, "--paste", "--tenancy", tTenancy, "--region", "us-phoenix-1", "--user-ocid", tUser,
		"--fingerprint", fpOf(t, &k.PublicKey), "--compartment-ocid", tComp, "--json")
	var e jsonError
	if code != exitValidation || json.Unmarshal([]byte(out), &e) != nil || e.Code != errCodeValidation || e.ExitCode != exitValidation {
		t.Fatalf("prompt not refused: %d %s", code, out)
	}
	code, out, _ = runConnect(t, deps, "--name", "Bad Name", "--json")
	if code != exitValidation || json.Unmarshal([]byte(out), &e) != nil || e.Error == "" {
		t.Fatalf("bad name: %d %s", code, out)
	}
}

func TestCloudConnectOCITimeoutFromEnv(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want time.Duration
	}{
		{"", defaultOCICommandTimeout},
		{"abc", defaultOCICommandTimeout},
		{"-5", defaultOCICommandTimeout},
		{" 30 ", 30 * time.Second},
	} {
		t.Setenv("APP_CLOUD_CONNECT_TIMEOUT_SECONDS", tc.env)
		if got := ociCommandTimeout(); got != tc.want {
			t.Errorf("env %q: timeout = %s, want %s", tc.env, got, tc.want)
		}
	}
}
