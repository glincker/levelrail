package imagemove

import "testing"

func TestValidateImageRef(t *testing.T) {
	cases := []struct {
		ref string
		ok  bool
	}{
		{"abcdefghijklmnopqrstuvwx:1111111111111111111111111111111111111111", true},
		{"myapp:latest", true},
		{"registry.local:5000/team/app:v1.2", true},
		{"nginx", true},
		{"", false},
		{"-o ProxyCommand=sh", false},
		{"--help", false},
		{"app;rm -rf /", false},
		{"app && id", false},
		{"app|id", false},
		{"app$(id)", false},
		{"app`id`", false},
		{"app\nid", false},
		{"app id", false},
		{"App:Latest", false},
		{"app'x", false},
		{"app@sha256:0000000000000000000000000000000000000000000000000000000000000000", false},
		{"../etc/passwd", false},
		{"a//b", false},
		{"app:", false},
	}
	for _, c := range cases {
		t.Run(c.ref, func(t *testing.T) {
			err := ValidateImageRef(c.ref)
			if (err == nil) != c.ok {
				t.Fatalf("ValidateImageRef(%q) err = %v, want ok=%v", c.ref, err, c.ok)
			}
		})
	}
}

func TestValidateImageID(t *testing.T) {
	good := "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		id string
		ok bool
	}{
		{good, true},
		{"", false},
		{"sha256:abc", false},
		{good + "0", false},
		{"sha512:" + good[7:], false},
		{"SHA256:" + good[7:], false},
	}
	for _, c := range cases {
		if err := ValidateImageID(c.id); (err == nil) != c.ok {
			t.Errorf("ValidateImageID(%q) err = %v, want ok=%v", c.id, err, c.ok)
		}
	}
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in   string
		port int
		want Target
		ok   bool
	}{
		{"root@old.example.com", 0, Target{"root", "old.example.com", 22}, true},
		{"root@old.example.com:2222", 0, Target{"root", "old.example.com", 2222}, true},
		{"root@old.example.com", 2200, Target{"root", "old.example.com", 2200}, true},
		{"root@old.example.com:2222", 2222, Target{"root", "old.example.com", 2222}, true},
		{"root@old.example.com:2222", 22, Target{}, false},
		{"deploy@10.0.0.5", 0, Target{"deploy", "10.0.0.5", 22}, true},
		{"deploy@[2001:db8::1]:2222", 0, Target{"deploy", "2001:db8::1", 2222}, true},
		{"deploy@[2001:db8::1]", 0, Target{"deploy", "2001:db8::1", 22}, true},
		{"deploy@2001:db8::1", 0, Target{"deploy", "2001:db8::1", 22}, true},
		{"old.example.com", 0, Target{}, false},
		{"@host", 0, Target{}, false},
		{"root@", 0, Target{}, false},
		{"-oProxyCommand=sh@host", 0, Target{}, false},
		{"root@-oProxyCommand=sh", 0, Target{}, false},
		{"root@host;id", 0, Target{}, false},
		{"root@host name", 0, Target{}, false},
		{"root@host:0", 0, Target{}, false},
		{"root@host:70000", 0, Target{}, false},
		{"root@host:22x", 0, Target{}, false},
		{"root@host", 70000, Target{}, false},
		{"ro ot@host", 0, Target{}, false},
		{"root@[10.0.0.1]", 0, Target{}, false},
		{"root@[::1", 0, Target{}, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseTarget(c.in, c.port)
			if (err == nil) != c.ok {
				t.Fatalf("ParseTarget(%q, %d) err = %v, want ok=%v", c.in, c.port, err, c.ok)
			}
			if c.ok && got != c.want {
				t.Fatalf("ParseTarget(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestSaveCommand(t *testing.T) {
	got, err := SaveCommand("myapp:abc")
	if err != nil || got != "docker save myapp:abc" {
		t.Fatalf("SaveCommand = %q, %v", got, err)
	}
	if _, err := SaveCommand("x;id"); err == nil {
		t.Fatal("SaveCommand accepted an injected ref")
	}
}
