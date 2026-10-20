package envredact

import "testing"

func TestSecrets(t *testing.T) {
	cases := []struct {
		name, in string
		secrets  []string
		want     string
	}{
		{"none", "plain", nil, "plain"},
		{"empty secret skipped", "plain", []string{""}, "plain"},
		{"one", "pw hunter2 here", []string{"hunter2"}, "pw [redacted] here"},
		{"several and repeated", "x1 b x1 y2", []string{"x1", "y2"}, "[redacted] b [redacted] [redacted]"},
	}
	for _, c := range cases {
		if got := Secrets(c.in, c.secrets...); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestURLUserinfo(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"https://tok@host/repo.git", "https://redacted@host/repo.git", false},
		{"https://u:p@host/x", "https://redacted@host/x", false},
		{"https://host/x", "https://host/x", false},
		{"http://[::1", "", true},
	}
	for _, c := range cases {
		got, err := URLUserinfo(c.in, "redacted")
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("URLUserinfo(%q) = %q, %v", c.in, got, err)
		}
	}
}

func TestURLQuery(t *testing.T) {
	keys := map[string]bool{"token": true}
	cases := []struct{ in, want string }{
		{"https://h/p?a=1", "https://h/p?a=1"},
		{"https://h/p?Token=abc&a=1", "https://h/p?Token=REDACTED&a=1"},
		{"http://[::1", "http://[::1"},
	}
	for _, c := range cases {
		if got := URLQuery(c.in, keys, "REDACTED"); got != c.want {
			t.Errorf("URLQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
