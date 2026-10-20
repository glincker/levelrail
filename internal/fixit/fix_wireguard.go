package fixit

import (
	"context"
	"fmt"
)

// CheckState is a check result reduced to what a fix needs.
type CheckState struct {
	Failing bool
	Detail  string
}

// WireGuardDeps are the host facts the privilege fix reasons over.
type WireGuardDeps struct {
	Check func(ctx context.Context) (CheckState, error)
	Host  func() HostEnv
	// FallbackUnit names the systemd unit when the cgroup does not.
	FallbackUnit string
}

// WireGuardPrivilegeFix returns the guided fix for a mesh blocked by privileges.
func WireGuardPrivilegeFix(d WireGuardDeps) Fix {
	return Fix{
		ID: "mesh.wireguard.privileges", CheckID: "wireguard", Subject: "mesh", Kind: KindGuided,
		Title: "Give the control plane permission to create the WireGuard interface",
		Why:   "WireGuard needs CAP_NET_ADMIN and /dev/net/tun. This process was started without them, so the mesh cannot come up.",
		Risk:  RiskRestarts, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			c, err := d.Check(ctx)
			if err != nil {
				return Applicability{}, err
			}
			a := Applicability{Failing: c.Failing, Detail: c.Detail, Changes: []Change{}}
			if !c.Failing {
				return a, nil
			}
			h := d.Host()
			a.Command, a.Changes, a.Reason = wireGuardGuidance(h, d.FallbackUnit)
			if a.Command == "" {
				a.Kind = KindManual
			}
			return a, nil
		},
	}
}

func wireGuardGuidance(h HostEnv, fallbackUnit string) (string, []Change, string) {
	var tun string
	if !h.TunPresent {
		tun = "sudo modprobe tun\n"
	}
	switch h.Mode {
	case RunSystemd:
		unit := h.Unit
		if unit == "" {
			unit = fallbackUnit
		}
		cmd := fmt.Sprintf("%ssudo mkdir -p /etc/systemd/system/%[2]s.service.d\n"+
			"printf '[Service]\\nAmbientCapabilities=CAP_NET_ADMIN\\nDeviceAllow=/dev/net/tun rw\\n' | sudo tee /etc/systemd/system/%[2]s.service.d/wireguard.conf\n"+
			"sudo systemctl daemon-reload && sudo systemctl restart %[2]s", tun, unit)
		return cmd, []Change{
			{Label: "Add a systemd drop-in", Detail: "/etc/systemd/system/" + unit + ".service.d/wireguard.conf"},
			{Label: "Restart the control plane", Detail: unit + ".service"},
		}, ""
	case RunDocker:
		cmd := tun + "docker run ... --cap-add NET_ADMIN --device /dev/net/tun ...\n# compose: cap_add: [NET_ADMIN] and devices: [\"/dev/net/tun:/dev/net/tun\"]"
		return cmd, []Change{
			{Label: "Recreate the container with NET_ADMIN", Detail: "--cap-add NET_ADMIN --device /dev/net/tun"},
		}, ""
	}
	if h.Root {
		return "", nil, "the process is already root, so the missing piece is the tun device or kernel support on this host"
	}
	exe := h.Executable
	if exe == "" {
		exe = "<control-plane-binary>"
	}
	return tun + "sudo " + exe, []Change{{Label: "Run the control plane as root", Detail: exe}}, ""
}
