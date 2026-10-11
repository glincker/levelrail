package compose

import "testing"

func TestResolveMagicVars_URLAndHostNameAService(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		svc     string
		want    string
		wantUnr bool
	}{
		{"url of another service", "${SERVICE_URL_WEB}", "worker", "https://web.example.com", false},
		{"host of another service", "${SERVICE_HOST_WEB}", "worker", "web.example.com", false},
		{"host embedded", "ssl://${SERVICE_HOST_MY_WEB}:443", "worker", "ssl://my-web.example.com:443", false},
		{"unknown key falls back to own domain", "${SERVICE_URL_NOPE}", "web", "https://web.example.com", false},
		{"unknown key without own domain", "${SERVICE_URL_NOPE}", "worker", "", true},
		{"key names a service that has no domain", "${SERVICE_URL_WORKER}", "web", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `
x-levelrail-domains:
  web: web.example.com
  my-web: my-web.example.com
services:
  web:
    image: web:1
  my-web:
    image: web:1
  worker:
    image: worker:1
`
			f, err := Parse([]byte(body))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			svc := f.Services[tt.svc]
			svc.Environment = Environment{"V": tt.token}
			f.Services[tt.svc] = svc
			_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
			if err != nil {
				t.Fatalf("ResolveMagicVars() error = %v", err)
			}
			if tt.wantUnr {
				if len(unresolved) != 1 {
					t.Fatalf("unresolved = %+v, want one", unresolved)
				}
				return
			}
			if len(unresolved) != 0 {
				t.Fatalf("unresolved = %+v", unresolved)
			}
			if got := f.Services[tt.svc].Environment["V"]; got != tt.want {
				t.Errorf("V = %q, want %q", got, tt.want)
			}
		})
	}
}
