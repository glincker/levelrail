package api

import (
	"context"
	"crypto"
	"crypto/md5" //nolint:gosec // OCI fingerprints are MD5 by definition
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ocisign"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	oTenancy = "ocid1.tenancy.oc1..aaaaaaaa11111111"
	oUser    = "ocid1.user.oc1..bbbbbbbb22222222"
	oComp    = "ocid1.compartment.oc1..cccccccc33333333"
	oSubnet  = "ocid1.subnet.oc1.phx.dddddddd44444444"
	oInst    = "ocid1.instance.oc1.phx.eeeeeeee55555555"
)

var oAuthRe = regexp.MustCompile(`keyId="([^"]+)",algorithm="rsa-sha256",headers="([^"]+)",signature="([^"]+)"`)

// fakeOCIServer verifies every signature like OCI and counts launches.
func fakeOCIServer(t *testing.T, pub *rsa.PublicKey, fp string, launches *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := oAuthRe.FindStringSubmatch(r.Header.Get("Authorization"))
		okSig := m != nil && m[1] == oTenancy+"/"+oUser+"/"+fp && m[2] == strings.Join(ocisign.Headers(r.Method), " ")
		if okSig {
			sig, _ := base64.StdEncoding.DecodeString(m[3])
			d := sha256.Sum256([]byte(ocisign.SigningString(r)))
			okSig = rsa.VerifyPKCS1v15(pub, crypto.SHA256, d[:], sig) == nil
		}
		if !okSig {
			http.Error(w, `{"code":"NotAuthenticated"}`, http.StatusUnauthorized)
			return
		}
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/regionSubscriptions"):
			_, _ = w.Write([]byte(`[{"regionName":"us-phoenix-1","status":"READY"}]`))
		case strings.HasSuffix(p, "/availabilityDomains"):
			_, _ = w.Write([]byte(`[{"name":"x:PHX-AD-1"}]`))
		case strings.HasSuffix(p, "/shapes"):
			_, _ = w.Write([]byte(`[{"shape":"VM.Standard.A1.Flex","ocpuOptions":{"min":1,"max":80},"memoryOptions":{"minInGBs":1,"maxInGBs":512}}]`))
		case strings.HasSuffix(p, "/compartments"):
			w.WriteHeader(http.StatusForbidden)
		case p == "/instances" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[{"shape":"VM.Standard.A1.Flex","lifecycleState":"RUNNING","shapeConfig":{"ocpus":1,"memoryInGBs":6}}]`))
		case strings.HasSuffix(p, "/images"):
			_, _ = w.Write([]byte(`[{"id":"ocid1.image.oc1.phx.ffffffff66666666"}]`))
		case p == "/instances" && r.Method == http.MethodPost:
			launches.Add(1)
			_, _ = w.Write([]byte(`{"id":"` + oInst + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOracleConnectAndCreateEndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	sum := md5.Sum(der) //nolint:gosec // OCI fingerprint format
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	fp := strings.Join(parts, ":")
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))

	var launches atomic.Int32
	srv := fakeOCIServer(t, &key.PublicKey, fp, &launches)
	secrets := newFakeNodeProviderSecrets()
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db,
		WithNodeProviderSecrets(secrets), WithAgentCAFingerprint("test-ca-fingerprint"))
	rt.oracleTestBase = srv.URL
	cookie := loginTestSession(t, rt, db)

	connect := func(fingerprint string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{
			"provider": "oracle", "method": "api_key", "verify": true, "tenancy_ocid": oTenancy, "user_ocid": oUser,
			"fingerprint": fingerprint, "private_key_pem": pemText, "region": "us-phoenix-1",
			"compartment_ocid": oComp, "subnet_ocid": oSubnet,
		})
		return provReq(rt, cookie, t, http.MethodPost, "/api/v1/node-providers", string(body))
	}
	bad := strings.Replace(fp, fp[:2], "00", 1)
	if rec := connect(bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatched fingerprint status = %d, want 400, body %s", rec.Code, rec.Body.String())
	}
	if ok, _ := secrets.Exists(context.Background(), store.NodeProviderSecretsKey("oracle"), store.NodeProviderTokenEnvKey); ok {
		t.Fatal("nothing may be sealed after a refused connect")
	}
	rec := connect(fp)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d, body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "PRIVATE KEY") || strings.Contains(rec.Body.String(), "private_key") {
		t.Fatalf("response leaked the key: %s", rec.Body.String())
	}

	create := func(name string) *httptest.ResponseRecorder {
		return provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision",
			`{"provider":"oracle","region":"us-phoenix-1","size":"VM.Standard.A1.Flex:1:6","name":"`+name+`","control_plane_addr":"cp.example.com:9443"}`)
	}
	if rec := create("ora-1"); rec.Code != http.StatusConflict || launches.Load() != 0 {
		t.Fatalf("default off create = %d launches %d, want 409 and 0", rec.Code, launches.Load())
	}
	setSetting(t, rt, runtimesettings.KeyProvisionOracleEnabled, `true`)
	setSetting(t, rt, runtimesettings.KeyProvisionOracleAllowUnknown, `true`)
	setSetting(t, rt, runtimesettings.KeyProvisionOracleMaxA1OCPU, `1`)
	setSetting(t, rt, runtimesettings.KeyProvisionOracleMaxA1MemoryGB, `6`)
	if rec := create("ora-1"); rec.Code != http.StatusConflict || launches.Load() != 0 {
		t.Fatalf("over allowance create = %d launches %d, want 409 and 0", rec.Code, launches.Load())
	}
	setSetting(t, rt, runtimesettings.KeyProvisionOracleMaxA1OCPU, `2`)
	setSetting(t, rt, runtimesettings.KeyProvisionOracleMaxA1MemoryGB, `12`)
	rec = create("ora-1")
	if rec.Code != http.StatusCreated || launches.Load() != 1 {
		t.Fatalf("create = %d launches %d, body %s", rec.Code, launches.Load(), rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "PRIVATE KEY") {
		t.Fatal("create response leaked the key")
	}
}
