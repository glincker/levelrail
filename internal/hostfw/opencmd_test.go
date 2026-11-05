package hostfw

import (
	"strings"
	"testing"
)

func TestOpenCommandsPerBackend(t *testing.T) {
	tests := []struct {
		b    Backend
		want string
	}{
		{BackendUFW, "ufw allow 8080/tcp"},
		{BackendFirewalld, "firewall-cmd --permanent --add-port=8080/tcp && firewall-cmd --reload"},
		{BackendNftables, "nft add rule inet filter input tcp dport 8080 accept"},
		{BackendIptables, "iptables -I INPUT -p tcp --dport 8080 -j ACCEPT"},
	}
	for _, tc := range tests {
		got := OpenCommands(tc.b, []int{8080})
		if len(got) != 1 || got[0].Command != tc.want || got[0].Port != 8080 || got[0].Backend != tc.b || got[0].Note == "" {
			t.Errorf("%s: got %+v, want %q", tc.b, got, tc.want)
		}
	}
}

func TestOpenCommandsPortValidation(t *testing.T) {
	got := OpenCommands(BackendUFW, []int{0, -1, 65536, 22, 22, 443})
	if len(got) != 2 || got[0].Port != 22 || got[1].Port != 443 {
		t.Fatalf("got %+v, want ports 22 and 443 once", got)
	}
	for _, c := range got {
		if strings.ContainsAny(c.Command, ";|`$\n") {
			t.Errorf("shell metacharacter in %q", c.Command)
		}
	}
	if len(OpenCommands(BackendNone, []int{99999})) != 0 {
		t.Error("invalid port produced commands")
	}
}

func TestOpenCommandsNoneListsAllFour(t *testing.T) {
	got := OpenCommands(BackendNone, []int{80})
	if len(got) != 4 {
		t.Fatalf("got %d, want 4", len(got))
	}
	for _, c := range got {
		if !strings.Contains(c.Note, "option for "+string(c.Backend)) {
			t.Errorf("note %q not labelled for %s", c.Note, c.Backend)
		}
	}
}

func TestOpenFixPerBackend(t *testing.T) {
	web := []int{80, 443}
	tests := []struct {
		b    Backend
		want string
	}{
		{BackendUFW, "sudo ufw allow 80/tcp && sudo ufw allow 443/tcp"},
		{BackendFirewalld, "sudo firewall-cmd --permanent --add-port=80/tcp --add-port=443/tcp && sudo firewall-cmd --reload"},
		{BackendNftables, "sudo nft add rule inet filter input tcp dport 80 accept && sudo nft add rule inet filter input tcp dport 443 accept"},
		{BackendIptables, "sudo iptables -I INPUT -p tcp --dport 80 -j ACCEPT && sudo iptables -I INPUT -p tcp --dport 443 -j ACCEPT"},
	}
	for _, tc := range tests {
		if got := OpenFix(tc.b, web); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.b, got, tc.want)
		}
	}
	if OpenFix(BackendUFW, []int{0}) != "" {
		t.Error("invalid port produced a fix")
	}
}
