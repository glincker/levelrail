package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretprovider"
)

func TestSecretProviderResourceNeverSerializesToken(t *testing.T) {
	cfg := secretprovider.Config{Name: "dp", Type: secretprovider.TypeDoppler, Project: "p", Environment: "e", GraceUntil: time.Now().Add(time.Hour)}
	res := toSecretProviderResource(cfg, true)
	for _, v := range []any{res, secretProviderListResponse{Providers: []secretProviderResource{res}}, secretProviderTestResponse{}} {
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if strings.Contains(tag, "token") && tag != "has_token" {
				t.Fatalf("%s.%s serializes %q", typ.Name(), typ.Field(i).Name, tag)
			}
		}
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"token"`) {
			t.Fatalf("response carries a token field: %s", raw)
		}
	}
}

func TestSecretProviderOnePasswordNeedsNoProjectOrEnvironment(t *testing.T) {
	cfg := secretprovider.Config{Name: "vaults", Type: secretprovider.TypeOnePassword, BaseURL: "https://connect.internal:8080"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("onepassword without project and environment: %v", err)
	}
	cfg.BaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("onepassword without base_url must fail")
	}
}
