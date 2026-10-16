package fixit

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$`)

// PublicAddressDeps are the facts and the setter the address fix uses.
type PublicAddressDeps struct {
	Check      func(ctx context.Context) (CheckState, error)
	Candidates func(ctx context.Context) []Option
	Save       func(ctx context.Context, host, actor string) error
}

// ValidAdvertiseHost accepts a hostname or a non-loopback IP, never a URL or port.
func ValidAdvertiseHost(h string) error {
	h = strings.TrimSpace(h)
	if h == "" {
		return fmt.Errorf("a host is required")
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("%s cannot be reached from another machine", h)
		}
		return nil
	}
	if strings.EqualFold(h, "localhost") || !hostnamePattern.MatchString(h) {
		return fmt.Errorf("%q is not a hostname or IP address (no scheme or port)", h)
	}
	return nil
}

// PublicAddressFix persists the address agents dial, overriding the env var.
func PublicAddressFix(d PublicAddressDeps) Fix {
	return Fix{
		ID: "mesh.public_address.set", CheckID: "public_address", Subject: "mesh", Kind: KindAuto,
		Title: "Set this server's public address",
		Why:   "Other nodes dial this address to enroll and to reach the WireGuard hub. Without one they cannot find the control plane.",
		Risk:  RiskSafe, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			c, err := d.Check(ctx)
			if err != nil {
				return Applicability{}, err
			}
			a := Applicability{Failing: c.Failing, Detail: c.Detail, Changes: []Change{}}
			if !c.Failing {
				return a, nil
			}
			opts := d.Candidates(ctx)
			def := ""
			if len(opts) > 0 {
				def = opts[0].Value
			}
			a.Param = &Param{Name: "host", Options: opts, Default: def, Free: true}
			a.Changes = []Change{
				{Label: "Save the address as a setting", Detail: "overrides APP_AGENT_ADVERTISE_HOST, no restart needed"},
				{Label: "Takes effect for new join commands and readiness checks now; the agent TLS certificate and mesh hub endpoint pick it up at the next restart"},
			}
			return a, nil
		},
		Run: func(ctx context.Context, in Input) error {
			host := strings.TrimSpace(in.Params["host"])
			if err := ValidAdvertiseHost(host); err != nil {
				return err
			}
			return d.Save(ctx, host, in.Actor)
		},
	}
}
