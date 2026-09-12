package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

func TestTLSMaterialFor_NilSecretsManager_ReturnsNil(t *testing.T) {
	material, err := tlsMaterialFor(context.Background(), nil, "main")
	if err != nil {
		t.Fatalf("tlsMaterialFor() error = %v", err)
	}
	if material != nil {
		t.Errorf("material = %+v, want nil when mgr is nil", material)
	}
}

func TestTLSMaterialFor_GeneratesOnFirstCall(t *testing.T) {
	mgr := newTestSecretsManager(t)

	material, err := tlsMaterialFor(context.Background(), mgr, "main")
	if err != nil {
		t.Fatalf("tlsMaterialFor() error = %v", err)
	}
	if material == nil {
		t.Fatal("material = nil, want a generated certificate")
	}

	certBlock, _ := pem.Decode(material.CertPEM)
	if certBlock == nil {
		t.Fatal("CertPEM did not decode to a PEM block")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if want := database.ContainerName("main"); cert.Subject.CommonName != want {
		t.Errorf("Subject.CommonName = %q, want %q", cert.Subject.CommonName, want)
	}
}

func TestTLSMaterialFor_ReusesExistingMaterialOnSecondCall(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	first, err := tlsMaterialFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("first tlsMaterialFor() error = %v", err)
	}
	second, err := tlsMaterialFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("second tlsMaterialFor() error = %v", err)
	}
	if string(first.CertPEM) != string(second.CertPEM) {
		t.Error("second call regenerated a different certificate; TLS material must be generated once and reused")
	}
	if string(first.KeyPEM) != string(second.KeyPEM) {
		t.Error("second call regenerated a different key; TLS material must be generated once and reused")
	}
}

func TestTLSMaterialFor_DifferentDatabasesGetDifferentMaterial(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	main, err := tlsMaterialFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("tlsMaterialFor(main) error = %v", err)
	}
	analytics, err := tlsMaterialFor(ctx, mgr, "analytics")
	if err != nil {
		t.Fatalf("tlsMaterialFor(analytics) error = %v", err)
	}
	if string(main.CertPEM) == string(analytics.CertPEM) {
		t.Error("two different databases got the same certificate")
	}
}
