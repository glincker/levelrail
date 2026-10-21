package localmode

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

const (
	// DefaultDashboardPort matches the CLI's default API address.
	DefaultDashboardPort = 8080
	maxPortProbes        = 20
)

// Ports are the loopback ports a local instance listens on.
type Ports struct {
	Dashboard int
	HTTP      int
	HTTPS     int
	Agent     int
}

// PortFree reports whether port can be bound on loopback right now.
type PortFree func(port int) bool

// LoopbackFree is the real PortFree: a listen-and-close probe.
func LoopbackFree(port int) bool {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// ChoosePorts picks the dashboard port (fixed when requested, else the first free one from 8080)
// and ingress ports (80 and 443 when free, else the first free high ports).
func ChoosePorts(requestedDashboard int, free PortFree) (Ports, error) {
	var p Ports
	used := map[int]bool{}
	pick := func(preferred []int, from int) (int, error) {
		for _, c := range preferred {
			if !used[c] && free(c) {
				used[c] = true
				return c, nil
			}
		}
		for c := from; c < from+maxPortProbes; c++ {
			if !used[c] && free(c) {
				used[c] = true
				return c, nil
			}
		}
		return 0, fmt.Errorf("localmode: no free port near %d", from)
	}
	var err error
	if requestedDashboard != 0 {
		if !free(requestedDashboard) {
			return p, fmt.Errorf("localmode: port %d is already in use; pick another with --port", requestedDashboard)
		}
		used[requestedDashboard] = true
		p.Dashboard = requestedDashboard
	} else if p.Dashboard, err = pick(nil, DefaultDashboardPort); err != nil {
		return p, err
	}
	if p.HTTP, err = pick([]int{80}, 8081); err != nil {
		return p, err
	}
	if p.HTTPS, err = pick([]int{443}, 8443); err != nil {
		return p, err
	}
	if p.Agent, err = pick([]int{9443}, 9444); err != nil {
		return p, err
	}
	return p, nil
}
