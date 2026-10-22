package dnszones

import (
	"strings"
	"testing"
)

func TestParseGoogleCloudKey(t *testing.T) {
	const secretMarker = "SECRET-MARKER"
	tests := []struct {
		name    string
		raw     string
		want    GoogleCloudIdentity
		wantErr string
	}{
		{"valid", `{"type":"service_account","project_id":"p1","client_email":"sa@p1.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\n` + secretMarker + `"}`,
			GoogleCloudIdentity{ProjectID: "p1", ClientEmail: "sa@p1.iam.gserviceaccount.com"}, ""},
		{"not json", `hello ` + secretMarker, GoogleCloudIdentity{}, "must be a JSON file"},
		{"wrong type", `{"type":"authorized_user","project_id":"p1"}`, GoogleCloudIdentity{}, "service_account"},
		{"no project", `{"type":"service_account","client_email":"a@b","private_key":"PRIVATE KEY"}`, GoogleCloudIdentity{}, "project_id"},
		{"no email", `{"type":"service_account","project_id":"p1","private_key":"PRIVATE KEY"}`, GoogleCloudIdentity{}, "client_email"},
		{"no private key", `{"type":"service_account","project_id":"p1","client_email":"a@b"}`, GoogleCloudIdentity{}, "private_key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseGoogleCloudKey(tc.raw)
			if tc.wantErr == "" {
				if err != nil || got != tc.want {
					t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if strings.Contains(err.Error(), secretMarker) {
				t.Fatalf("error leaks the key: %v", err)
			}
		})
	}
}
