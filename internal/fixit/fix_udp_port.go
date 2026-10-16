package fixit

import (
	"context"
	"fmt"
)

// PortHolder names what is bound to a port.
type PortHolder struct {
	Process   string
	PID       int
	Container string
}

// UDPPortDeps are the facts the busy-port fix needs.
type UDPPortDeps struct {
	Check  func(ctx context.Context) (CheckState, error)
	Port   int
	Holder func(ctx context.Context, port int) (PortHolder, bool)
}

// UDPPortFix names the holder of the mesh UDP port and how to free it. The
// port is a compiled default today, so there is no "pick another port" tier.
func UDPPortFix(d UDPPortDeps) Fix {
	return Fix{
		ID: "mesh.udp_port.holder", CheckID: "udp_port", Subject: "mesh", Kind: KindGuided,
		Title: fmt.Sprintf("Free UDP port %d", d.Port),
		Why:   "The WireGuard mesh listens on one fixed UDP port. Another process already holds it.",
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
			h, ok := d.Holder(ctx, d.Port)
			switch {
			case ok && h.Container != "":
				a.Changes = []Change{{Label: "Stop the container using the port", Detail: h.Container}}
				a.Command = "docker stop " + h.Container
			case ok && h.PID > 0:
				a.Changes = []Change{{Label: "Stop the process using the port", Detail: fmt.Sprintf("%s (pid %d)", h.Process, h.PID)}}
				a.Command = fmt.Sprintf("sudo systemctl stop %s  # or: sudo kill %d", h.Process, h.PID)
			default:
				a.Command = fmt.Sprintf("sudo ss -ulpn | grep %d", d.Port)
				a.Reason = "the holder could not be identified from this process"
			}
			return a, nil
		},
	}
}
