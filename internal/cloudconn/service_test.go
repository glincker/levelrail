package cloudconn

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newSvc(t *testing.T, reg Registry) *Service {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Service{Rows: db, Providers: reg}
}

func ociInput() Input {
	return Input{
		Provider: ProviderOracle, AuthMethod: AuthSecretRef, CredentialRef: "secret://vault/oci#key",
		Config: Config{Region: "us-ashburn-1", AccountID: "ocid1.tenancy.x", Compartment: "ocid1.compartment.x"},
	}
}

func TestCreateValidation(t *testing.T) {
	s := newSvc(t, DefaultRegistry())
	ctx := context.Background()
	cases := map[string]func(n *string, in *Input){
		"uppercase name":      func(n *string, _ *Input) { *n = "Prod" },
		"bad provider":        func(_ *string, in *Input) { in.Provider = "nope" },
		"bad auth":            func(_ *string, in *Input) { in.AuthMethod = "magic" },
		"auth not for vendor": func(_ *string, in *Input) { in.Provider, in.AuthMethod = ProviderHetzner, AuthOIDCFederation },
		"missing region":      func(_ *string, in *Input) { in.Config.Region = "" },
		"missing ref":         func(_ *string, in *Input) { in.CredentialRef = "" },
		"malformed ref":       func(_ *string, in *Input) { in.CredentialRef = "not a ref" },
		"oidc with ref":       func(_ *string, in *Input) { in.AuthMethod = AuthOIDCFederation },
		"control chars":       func(_ *string, in *Input) { in.Config.Compartment = "a\nb" },
		"static bad shape":    func(_ *string, in *Input) { in.AuthMethod, in.CredentialRef = AuthStatic, "secret://v/p#k" },
	}
	for name, mut := range cases {
		n, in := "ok-name", ociInput()
		mut(&n, &in)
		if _, err := s.Create(ctx, "u", n, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := s.Create(ctx, "u", "ok-name", ociInput()); err != nil {
		t.Fatalf("valid create: %v", err)
	}
	if _, err := s.Create(ctx, "u", "ok-name", ociInput()); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate err = %v", err)
	}
}

func TestUpdateKeepsAndChanges(t *testing.T) {
	s := newSvc(t, DefaultRegistry())
	ctx := context.Background()
	if _, err := s.Create(ctx, "u", "oci", ociInput()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Update(ctx, "oci", Input{TeamID: "team1"})
	if err != nil || got.TeamID != "team1" || got.CredentialRef != "secret://vault/oci#key" || got.Config.Region != "us-ashburn-1" {
		t.Fatalf("partial update = %+v, %v", got, err)
	}
	if _, err := s.Update(ctx, "oci", Input{Provider: ProviderAWS}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("provider change err = %v", err)
	}
	if _, err := s.Update(ctx, "oci", Input{AuthMethod: AuthOIDCFederation}); err != nil {
		t.Fatalf("switch to oidc: %v", err)
	}
	got, _ = s.Get(ctx, "oci")
	if got.CredentialRef != "" {
		t.Fatalf("oidc connection kept a credential ref: %q", got.CredentialRef)
	}
	if _, err := s.Update(ctx, "missing", Input{TeamID: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestTestViaFakeProvider(t *testing.T) {
	reg, fakes := FakeRegistry(ClassOK, nil)
	s := newSvc(t, reg)
	ctx := context.Background()
	in := ociInput()
	in.AuthMethod, in.CredentialRef = AuthOIDCFederation, ""
	if _, err := s.Create(ctx, "u", "oci", in); err != nil {
		t.Fatal(err)
	}
	res, err := s.Test(ctx, "oci")
	if err != nil || !res.OK || res.Class != ClassOK || res.Reason == "" {
		t.Fatalf("test = %+v, %v", res, err)
	}
	if got := fakes[ProviderOracle].Checked; len(got) != 1 || got[0] != "oci" {
		t.Fatalf("fake checked %v", got)
	}
	c, _ := s.Get(ctx, "oci")
	if c.LastCheckClass != ClassOK || c.LastCheckedAt.IsZero() {
		t.Fatalf("check not recorded: %+v", c)
	}

	fakes[ProviderOracle].Class, fakes[ProviderOracle].Err = ClassAuthFailed, errors.New("provider said: secret-token-xyz denied")
	res, _ = s.Test(ctx, "oci")
	if res.OK || res.Class != ClassAuthFailed || strings.Contains(res.Reason, "secret-token-xyz") {
		t.Fatalf("failure result leaked or wrong: %+v", res)
	}
	fakes[ProviderOracle].Class, fakes[ProviderOracle].Err = "made-up", nil
	if res, _ = s.Test(ctx, "oci"); res.Class != ClassUnknown {
		t.Fatalf("unknown class = %q", res.Class)
	}
}

func TestDefaultProvidersAreUnsupportedAndUnresolvedRefsClassified(t *testing.T) {
	s := newSvc(t, DefaultRegistry())
	ctx := context.Background()
	if _, err := s.Create(ctx, "u", "oci", ociInput()); err != nil {
		t.Fatal(err)
	}
	if res, _ := s.Test(ctx, "oci"); res.Class != ClassCredentialUnresolved {
		t.Fatalf("no sources: class = %q", res.Class)
	}
	in := ociInput()
	in.AuthMethod, in.CredentialRef = AuthOIDCFederation, ""
	if _, err := s.Create(ctx, "u", "oidc", in); err != nil {
		t.Fatal(err)
	}
	res, err := s.Test(ctx, "oidc")
	if err != nil || res.OK || res.Class != ClassUnsupported {
		t.Fatalf("default provider test = %+v, %v", res, err)
	}
	if _, err := DefaultRegistry()[ProviderOracle].ListRegions(ctx, Target{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("regions err = %v", err)
	}
}
