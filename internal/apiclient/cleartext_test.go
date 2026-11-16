package apiclient

import "testing"

func TestCheckCleartextHTTP(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	envOn := func(k string) (string, bool) { return "1", k == EnvInsecureHTTP }
	cases := []struct {
		name    string
		url     string
		allow   bool
		env     func(string) (string, bool)
		wantErr bool
	}{
		{"localhost http", "http://localhost:8080", false, none, false},
		{"127 range http", "http://127.0.0.5:8080", false, none, false},
		{"ipv6 loopback http", "http://[::1]:8080", false, none, false},
		{"remote http rejected", "http://example.com:8080", false, none, true},
		{"remote ip http rejected", "http://10.0.0.5", false, none, true},
		{"remote https ok", "https://example.com", false, none, false},
		{"flag allows http", "http://example.com", true, none, false},
		{"env allows http", "http://example.com", false, envOn, false},
	}
	for _, tc := range cases {
		err := CheckCleartextHTTP(tc.url, tc.allow, tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}
