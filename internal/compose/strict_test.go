package compose

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func strictCodes(t *testing.T, err error) []string {
	t.Helper()
	var se StrictErrors
	if !errors.As(err, &se) {
		t.Fatalf("want StrictErrors, got %T: %v", err, err)
	}
	out := make([]string, len(se))
	for i, e := range se {
		out[i] = e.Code + "/" + e.Key + "/" + e.Rule
	}
	return out
}

func TestStrictRefusals(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{"bind mount", "services:\n  web:\n    image: nginx\n    volumes:\n      - /etc:/data\n", "compose_rejected/volumes/bind_undeclared"},
		{"build", "services:\n  web:\n    build: .\n", "compose_unsupported_key/build/"},
		{"env_file", "services:\n  web:\n    image: nginx\n    env_file: .env\n", "compose_unsupported_key/env_file/"},
		{"extends file", "services:\n  web:\n    image: nginx\n    extends:\n      file: other.yaml\n      service: base\n", "compose_unsupported_key/extends/"},
		{"include", "include:\n  - other.yaml\nservices:\n  web:\n    image: nginx\n", "compose_unsupported_key/include/"},
		{"unknown top level", "secrets:\n  a:\n    file: ./a\nservices:\n  web:\n    image: nginx\n", "compose_unsupported_key/secrets/"},
		{"host-gateway", "services:\n  web:\n    image: nginx\n    extra_hosts:\n      - \"h:host-gateway\"\n", "compose_rejected/extra_hosts/"},
		{"metadata", "services:\n  web:\n    image: nginx\n    extra_hosts:\n      - \"m:169.254.169.254\"\n", "compose_rejected/extra_hosts/"},
		{"loopback v6", "services:\n  web:\n    image: nginx\n    extra_hosts:\n      m: \"::1\"\n", "compose_rejected/extra_hosts/"},
		{"privileged", "services:\n  web:\n    image: nginx\n    privileged: true\n", "compose_rejected//privileged"},
		{"cap_add", "services:\n  web:\n    image: nginx\n    cap_add: [NET_ADMIN]\n", "compose_rejected//cap_add_disallowed"},
		{"docker socket", "services:\n  web:\n    image: nginx\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n", "compose_rejected/volumes/bind_undeclared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseStrict([]byte(tc.yaml), StrictLimits{MaxBytes: 1 << 16, MaxNodes: 10000})
			if err == nil {
				t.Fatal("want refusal")
			}
			got := strictCodes(t, err)
			for _, g := range got {
				if g == tc.want {
					return
				}
			}
			t.Fatalf("want %s in %v", tc.want, got)
		})
	}
}

func TestStrictAcceptsPlainFile(t *testing.T) {
	src := "name: shop\nversion: \"3.8\"\nx-common: &c\n  restart: always\nservices:\n  web:\n    <<: *c\n    image: nginx\n    extra_hosts: [\"db:10.0.0.5\"]\n    volumes: [\"data:/data\"]\nvolumes:\n  data: {}\n"
	f, err := ParseStrict([]byte(src), StrictLimits{MaxBytes: 1 << 16, MaxNodes: 10000})
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if _, ok := f.Services["web"]; !ok {
		t.Fatal("web service missing")
	}
}

func TestStrictTooLarge(t *testing.T) {
	_, err := ParseStrict([]byte(strings.Repeat("#", 2048)), StrictLimits{MaxBytes: 1024, MaxNodes: 10000})
	if got := strictCodes(t, err); got[0] != "compose_too_large//" {
		t.Fatalf("got %v", got)
	}
}

func TestStrictAliasBomb(t *testing.T) {
	var b strings.Builder
	b.WriteString("x-a0: &a0 [\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\"]\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&b, "x-a%d: &a%d [", i, i)
		for j := 0; j < 10; j++ {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "*a%d", i-1)
		}
		b.WriteString("]\n")
	}
	b.WriteString("services:\n  web:\n    image: nginx\n    labels: *a9\n")
	start := time.Now()
	_, err := ParseStrict([]byte(b.String()), StrictLimits{MaxBytes: 1 << 16, MaxNodes: 250000})
	if got := strictCodes(t, err); got[0] != "compose_too_large//" {
		t.Fatalf("got %v", got)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("alias bomb took %s", d)
	}
}

func TestStrictNonStrictUnchanged(t *testing.T) {
	src := []byte("services:\n  web:\n    image: nginx\n    volumes:\n      - /srv/site:/usr/share/nginx/html\n    env_file: .env\n")
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("non strict Validate must still allow a bind and env_file: %v", err)
	}
}
