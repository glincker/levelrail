package hostfw

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeHost struct {
	tools map[string]bool
	out   map[string]string
	errs  map[string]error
	ran   []string
	// after replaces an output once a mutating command ran.
	after map[string]string
}

func (f *fakeHost) deps() Deps {
	return Deps{
		LookPath: func(n string) (string, error) {
			if f.tools[n] {
				return "/usr/sbin/" + n, nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			key := name + " " + strings.Join(args, " ")
			f.ran = append(f.ran, key)
			if err, ok := f.errs[key]; ok {
				return []byte(f.out[key]), err
			}
			if len(f.ran) > 0 && f.after != nil && isMutation(key) {
				for k, v := range f.after {
					f.out[k] = v
				}
			}
			return []byte(f.out[key]), nil
		},
	}
}

func isMutation(k string) bool {
	return strings.HasPrefix(k, "ufw allow") || strings.Contains(k, "--add-service") || strings.HasPrefix(k, "iptables -I") || strings.Contains(k, "insert rule")
}

const ufwBlocked = "Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n\nTo                         Action      From\n--                         ------      ----\n22/tcp                     ALLOW IN    Anywhere\n22/tcp (v6)                ALLOW IN    Anywhere (v6)\n"
const ufwOpen = ufwBlocked + "80,443/tcp                 ALLOW IN    Anywhere\n"

func TestDetectVariants(t *testing.T) {
	cases := []struct {
		name      string
		host      fakeHost
		backend   Backend
		active    bool
		blocking  bool
		unknown   bool
		wantPorts []int
	}{
		{"ufw active without web rules", fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": ufwBlocked}}, BackendUFW, true, true, false, []int{80, 443}},
		{"ufw active with rules", fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": ufwOpen}}, BackendUFW, true, false, false, nil},
		{"ufw app profile", fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": ufwBlocked + "Nginx Full                 ALLOW IN    Anywhere\n"}}, BackendUFW, true, false, false, nil},
		{"ufw rule limited to a subnet", fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": ufwBlocked + "80/tcp                     ALLOW IN    10.0.0.0/8\n443/tcp                    ALLOW IN    Anywhere\n"}}, BackendUFW, true, true, false, []int{80}},
		{"ufw inactive", fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": "Status: inactive\n"}}, BackendUFW, false, false, false, nil},
		{"ufw default allow", fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": "Status: active\nDefault: allow (incoming), allow (outgoing)\n\n--\n"}}, BackendUFW, false, false, false, nil},
		{"ufw needs root", fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": "ERROR: You need to be root to run this script"}, errs: map[string]error{"ufw status verbose": errors.New("exit status 1")}}, BackendUFW, false, false, true, nil},
		{"firewalld missing https", fakeHost{tools: map[string]bool{"firewall-cmd": true}, out: map[string]string{"firewall-cmd --state": "running", "firewall-cmd --query-service=http": "yes"}}, BackendFirewalld, true, true, false, []int{443}},
		{"firewalld not running", fakeHost{tools: map[string]bool{"firewall-cmd": true}, out: map[string]string{"firewall-cmd --state": "not running"}, errs: map[string]error{"firewall-cmd --state": errors.New("exit status 252")}}, BackendFirewalld, false, false, false, nil},
		{"nftables only, drop policy", fakeHost{tools: map[string]bool{"nft": true}, out: map[string]string{"nft list ruleset": "table inet filter {\n\tchain input {\n\t\ttype filter hook input priority filter; policy drop;\n\t\ttcp dport 22 accept\n\t}\n}\n"}}, BackendNftables, true, true, false, []int{80, 443}},
		{"nftables accepting web", fakeHost{tools: map[string]bool{"nft": true}, out: map[string]string{"nft list ruleset": "table inet filter {\n\tchain input {\n\t\ttype filter hook input priority filter; policy drop;\n\t\ttcp dport { 80, 443 } accept\n\t}\n}\n"}}, BackendNftables, true, false, false, nil},
		{"nftables without drop", fakeHost{tools: map[string]bool{"nft": true}, out: map[string]string{"nft list ruleset": "table inet filter {\n\tchain input {\n\t\ttype filter hook input priority filter; policy accept;\n\t}\n}\n"}}, BackendNftables, false, false, false, nil},
		{"iptables drop policy", fakeHost{tools: map[string]bool{"iptables": true}, out: map[string]string{"iptables -S INPUT": "-P INPUT DROP\n-A INPUT -p tcp -m tcp --dport 22 -j ACCEPT\n"}}, BackendIptables, true, true, false, []int{80, 443}},
		{"iptables trailing reject", fakeHost{tools: map[string]bool{"iptables": true}, out: map[string]string{"iptables -S INPUT": "-P INPUT ACCEPT\n-A INPUT -p tcp -m multiport --dports 80,443 -j ACCEPT\n-A INPUT -j REJECT --reject-with icmp-host-prohibited\n"}}, BackendIptables, true, false, false, nil},
		{"iptables accept policy", fakeHost{tools: map[string]bool{"iptables": true}, out: map[string]string{"iptables -S INPUT": "-P INPUT ACCEPT\n"}}, BackendIptables, false, false, false, nil},
		{"no firewall", fakeHost{tools: map[string]bool{}}, BackendNone, false, false, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := Detect(context.Background(), tc.host.deps())
			if st.Backend != tc.backend || st.Active != tc.active || st.Blocking() != tc.blocking || st.Unknown != tc.unknown {
				t.Fatalf("got backend=%s active=%v blocking=%v unknown=%v detail=%q", st.Backend, st.Active, st.Blocking(), st.Unknown, st.Detail)
			}
			got := st.MissingPorts()
			if len(got) != len(tc.wantPorts) {
				t.Fatalf("missing ports = %v, want %v", got, tc.wantPorts)
			}
		})
	}
}

func TestPlanAndApply(t *testing.T) {
	t.Run("ufw opens only the missing ports and reports undo", func(t *testing.T) {
		h := &fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": ufwBlocked}, after: map[string]string{"ufw status verbose": ufwOpen}}
		res, err := Apply(context.Background(), h.deps())
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"ufw status verbose", "ufw allow 80/tcp", "ufw allow 443/tcp"}
		for i, w := range want {
			if h.ran[i] != w {
				t.Fatalf("ran[%d] = %q, want %q (all: %v)", i, h.ran[i], w, h.ran)
			}
		}
		if len(res.Undo) != 2 || res.Undo[0].String() != "ufw delete allow 80/tcp" {
			t.Fatalf("undo = %v", res.Undo)
		}
		for _, r := range h.ran {
			if strings.Contains(r, "enable") || strings.Contains(r, "reset") {
				t.Fatalf("ran forbidden command %q", r)
			}
		}
	})
	t.Run("inactive firewall is left alone", func(t *testing.T) {
		h := &fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": "Status: inactive\n"}}
		res, err := Apply(context.Background(), h.deps())
		if err != nil || len(res.Plan.Add) != 0 || res.Plan.Note == "" {
			t.Fatalf("err=%v plan=%+v", err, res.Plan)
		}
		if len(h.ran) != 2 {
			t.Fatalf("ran %v", h.ran)
		}
	})
	t.Run("explicit deny is refused", func(t *testing.T) {
		h := &fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": ufwBlocked + "80/tcp                     DENY IN     Anywhere\n"}}
		_, err := Apply(context.Background(), h.deps())
		if !errors.Is(err, ErrRefused) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unreadable firewall fails closed", func(t *testing.T) {
		h := &fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": "you need to be root"}, errs: map[string]error{"ufw status verbose": errors.New("exit status 1")}}
		_, err := Apply(context.Background(), h.deps())
		if !errors.Is(err, ErrRefused) || len(h.ran) != 1 {
			t.Fatalf("err=%v ran=%v", err, h.ran)
		}
	})
	t.Run("failure rolls back what was added", func(t *testing.T) {
		h := &fakeHost{tools: map[string]bool{"ufw": true}, out: map[string]string{"ufw status verbose": ufwBlocked}, errs: map[string]error{"ufw allow 443/tcp": errors.New("boom")}}
		_, err := Apply(context.Background(), h.deps())
		if err == nil || h.ran[len(h.ran)-1] != "ufw delete allow 80/tcp" {
			t.Fatalf("err=%v ran=%v", err, h.ran)
		}
	})
	t.Run("firewalld adds services and reloads", func(t *testing.T) {
		h := &fakeHost{tools: map[string]bool{"firewall-cmd": true}, out: map[string]string{"firewall-cmd --state": "running"},
			after: map[string]string{"firewall-cmd --query-service=http": "yes", "firewall-cmd --query-service=https": "yes"}}
		res, err := Apply(context.Background(), h.deps())
		if err != nil || len(res.Plan.Add) != 3 || res.Plan.Add[2].String() != "firewall-cmd --reload" {
			t.Fatalf("err=%v plan=%+v", err, res.Plan)
		}
	})
	t.Run("iptables inserts before the catch-all and is not persistent", func(t *testing.T) {
		h := &fakeHost{tools: map[string]bool{"iptables": true}, out: map[string]string{"iptables -S INPUT": "-P INPUT ACCEPT\n-A INPUT -j REJECT\n"},
			after: map[string]string{"iptables -S INPUT": "-P INPUT ACCEPT\n-A INPUT -p tcp --dport 80 -j ACCEPT\n-A INPUT -p tcp --dport 443 -j ACCEPT\n-A INPUT -j REJECT\n"}}
		res, err := Apply(context.Background(), h.deps())
		if err != nil || res.Plan.Persistent || res.Plan.Add[0].String() != "iptables -I INPUT 1 -p tcp --dport 80 -j ACCEPT" {
			t.Fatalf("err=%v plan=%+v", err, res.Plan)
		}
	})
	t.Run("nftables records the handle for undo", func(t *testing.T) {
		rules := "table inet filter {\n\tchain input {\n\t\ttype filter hook input priority filter; policy drop;\n\t}\n}\n"
		h := &fakeHost{tools: map[string]bool{"nft": true}, out: map[string]string{"nft list ruleset": rules,
			"nft -e -a insert rule inet filter input tcp dport 80 accept":  "# handle 7",
			"nft -e -a insert rule inet filter input tcp dport 443 accept": "# handle 8"},
			after: map[string]string{"nft list ruleset": strings.Replace(rules, "\t}\n}", "\t\ttcp dport { 80, 443 } accept\n\t}\n}", 1)}}
		res, err := Apply(context.Background(), h.deps())
		if err != nil || len(res.Undo) != 2 || res.Undo[0].String() != "nft delete rule inet filter input handle 7" {
			t.Fatalf("err=%v undo=%v", err, res.Undo)
		}
	})
}
