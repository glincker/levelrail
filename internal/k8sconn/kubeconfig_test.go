package k8sconn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

func testCertPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "t"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd})
}

func kubeconfigWith(clusterExtra, userBody string) string {
	return fmt.Sprintf(`current-context: c
contexts:
- name: c
  context: {cluster: k, user: u}
clusters:
- name: k
  cluster:
    server: https://10.0.0.1:6443
%s
users:
- name: u
  user:
%s
`, clusterExtra, userBody)
}

func TestParseKubeconfig(t *testing.T) {
	cert, key := testCertPEM(t)
	b64 := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
	ca := "    certificate-authority-data: " + b64(cert)
	tok := "    token: abc"

	refused := map[string]string{
		"exec":          kubeconfigWith(ca, "    exec: {command: aws}"),
		"auth-provider": kubeconfigWith(ca, "    auth-provider: {name: gcp}"),
		"ca file":       kubeconfigWith("    certificate-authority: /etc/ca.crt", tok),
		"token file":    kubeconfigWith(ca, "    tokenFile: /var/run/t"),
		"cert file":     kubeconfigWith(ca, "    client-certificate: /a\n    client-key: /b"),
		"insecure":      kubeconfigWith(ca+"\n    insecure-skip-tls-verify: true", tok),
		"proxy":         kubeconfigWith(ca+"\n    proxy-url: http://p:3128", tok),
		"basic":         kubeconfigWith(ca, "    username: a\n    password: b"),
	}
	for name, doc := range refused {
		_, err := ParseKubeconfig([]byte(doc), "", 0)
		if !errors.Is(err, ErrRefused) {
			t.Errorf("%s: want ErrRefused, got %v", name, err)
		}
	}

	got, err := ParseKubeconfig([]byte(kubeconfigWith(ca, tok)), "c", 0)
	if err != nil || got.Token != "abc" || got.Server != "https://10.0.0.1:6443" || len(got.CA) == 0 {
		t.Fatalf("token context: %+v %v", got, err)
	}
	pair := "    client-certificate-data: " + b64(cert) + "\n    client-key-data: " + b64(key)
	got, err = ParseKubeconfig([]byte(kubeconfigWith(ca, pair)), "", 0)
	if err != nil || got.ClientCert == nil || got.ClientKey == nil {
		t.Fatalf("client cert context: %+v %v", got, err)
	}
	if _, err := ParseKubeconfig([]byte(kubeconfigWith(ca, tok)), "missing", 0); err == nil {
		t.Error("unknown context accepted")
	}
	if _, err := ParseKubeconfig([]byte(kubeconfigWith(ca, tok)), "", 10); err == nil {
		t.Error("size cap not enforced")
	}
	// A refused user in another context must not block the chosen one.
	other := kubeconfigWith(ca, tok) + "- name: x\n  user:\n    exec: {command: aws}\n"
	if _, err := ParseKubeconfig([]byte(other), "", 0); err != nil {
		t.Errorf("unrelated exec user blocked import: %v", err)
	}
}

func TestKubeconfigRefusesImpersonation(t *testing.T) {
	_, err := ParseKubeconfig([]byte(kubeconfigWith("", "    token: t\n    as: admin")), "", 0)
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("as = %v, want ErrRefused", err)
	}
}

func TestKubeconfigCarriesTLSServerNameAndRefusesUserinfo(t *testing.T) {
	c, err := ParseKubeconfig([]byte(kubeconfigWith("    tls-server-name: kube.internal", "    token: t")), "", 0)
	if err != nil || c.ServerName != "kube.internal" {
		t.Fatalf("server name = %q, %v", c.ServerName, err)
	}
	bad := strings.Replace(kubeconfigWith("", "    token: t"), "https://10.0.0.1", "https://u:p@10.0.0.1", 1)
	if _, err := ParseKubeconfig([]byte(bad), "", 0); err == nil {
		t.Fatal("userinfo server accepted")
	}
}
