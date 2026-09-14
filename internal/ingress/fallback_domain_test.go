package ingress

import "testing"

func TestFallbackDomain_PublicIPv4(t *testing.T) {
	got, ok := FallbackDomain("203.0.113.5", "my-app")
	if !ok {
		t.Fatal("FallbackDomain() ok = false, want true for a public IPv4 address")
	}
	if want := "my-app.203-0-113-5.sslip.io"; got != want {
		t.Errorf("FallbackDomain() = %q, want %q", got, want)
	}
}

func TestFallbackDomain_PublicIPv6(t *testing.T) {
	got, ok := FallbackDomain("2001:db8::1", "my-app")
	if !ok {
		t.Fatal("FallbackDomain() ok = false, want true for a public IPv6 address")
	}
	if want := "my-app.2001-db8--1.sslip.io"; got != want {
		t.Errorf("FallbackDomain() = %q, want %q", got, want)
	}
}

func TestFallbackDomain_SanitizesServiceName(t *testing.T) {
	got, ok := FallbackDomain("203.0.113.5", "My_App Name!!")
	if !ok {
		t.Fatal("FallbackDomain() ok = false, want true")
	}
	if want := "my-app-name.203-0-113-5.sslip.io"; got != want {
		t.Errorf("FallbackDomain() = %q, want %q", got, want)
	}
}

func TestFallbackDomain_RejectsNonIPHost(t *testing.T) {
	tests := []string{"", "controlplane.example.com", "not an ip"}
	for _, host := range tests {
		if _, ok := FallbackDomain(host, "app"); ok {
			t.Errorf("FallbackDomain(%q, ...) ok = true, want false: not an IP literal", host)
		}
	}
}

func TestFallbackDomain_RejectsNonPublicIPs(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"private RFC1918", "10.0.0.5"},
		{"private RFC1918 192.168", "192.168.1.1"},
		{"loopback", "127.0.0.1"},
		{"link-local", "169.254.1.1"},
		{"unspecified", "0.0.0.0"},
		{"IPv6 loopback", "::1"},
		{"IPv6 unique local", "fd00::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := FallbackDomain(tt.ip, "app"); ok {
				t.Errorf("FallbackDomain(%q, ...) ok = true, want false: not publicly routable", tt.ip)
			}
		})
	}
}

func TestFallbackDomain_EmptyServiceNameAfterSanitizing(t *testing.T) {
	if _, ok := FallbackDomain("203.0.113.5", "!!!"); ok {
		t.Error("FallbackDomain() ok = true, want false: nothing left after sanitizing the name")
	}
}

func TestSanitizeDNSLabel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"my-app", "my-app"},
		{"My App", "my-app"},
		{"My_App__Name", "my-app-name"},
		{"--leading-and-trailing--", "leading-and-trailing"},
		{"UPPERCASE", "uppercase"},
		{"123", "123"},
		{"", ""},
		{"!!!", ""},
	}
	for _, tt := range tests {
		if got := sanitizeDNSLabel(tt.in); got != tt.want {
			t.Errorf("sanitizeDNSLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
