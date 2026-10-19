package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Readiness check ids and statuses, stable for the API and UI.
const (
	CheckWireGuard     = "wireguard"
	CheckUDPPort       = "udp_port"
	CheckFirewall      = "firewall"
	CheckPublicAddress = "public_address"
	CheckLocalNode     = "local_node"

	CheckPass = "pass"
	CheckWarn = "warn"
	CheckFail = "fail"
)

// Check is one readiness result. Only CheckFail blocks enabling.
type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// ReadinessEnv is the host evidence ReadinessChecks reasons over, injectable for tests.
type ReadinessEnv struct {
	Probe Probe
	// TunDevicePresent reports /dev/net/tun, which wireguard-go needs on Linux.
	TunDevicePresent func() bool
	// UDPPortFree tries to bind the UDP port; inUse is true on EADDRINUSE.
	UDPPortFree func(port int) (free bool, err error)
	// UFWActive reports whether ufw is enabled on this host.
	UFWActive func() bool
	// MeshRunning is true when the mesh is already up in this process.
	MeshRunning bool
	// AdvertiseHost is APP_AGENT_ADVERTISE_HOST as the control plane resolved it.
	AdvertiseHost string
	Port          int
}

// SystemReadinessEnv reads the real host.
func SystemReadinessEnv(advertiseHost string, meshRunning bool) ReadinessEnv {
	return ReadinessEnv{
		Probe: SystemProbe{},
		TunDevicePresent: func() bool {
			_, err := os.Stat("/dev/net/tun")
			return err == nil
		},
		UDPPortFree: func(port int) (bool, error) {
			pc, err := (&net.ListenConfig{}).ListenPacket(context.Background(), "udp", ":"+strconv.Itoa(port))
			if err != nil {
				if errors.Is(err, syscall.EADDRINUSE) {
					return false, nil
				}
				return false, err
			}
			_ = pc.Close()
			return true, nil
		},
		UFWActive: func() bool {
			b, err := os.ReadFile("/etc/ufw/ufw.conf")
			if err != nil {
				return false
			}
			for _, l := range strings.Split(string(b), "\n") {
				if strings.EqualFold(strings.ReplaceAll(strings.TrimSpace(l), " ", ""), "ENABLED=yes") {
					return true
				}
			}
			return false
		},
		MeshRunning:   meshRunning,
		AdvertiseHost: advertiseHost,
		Port:          DefaultListenPort,
	}
}

// ReadinessChecks answers "can this host run the mesh", with a fix hint for
// anything that cannot.
func ReadinessChecks(e ReadinessEnv) []Check {
	port := e.Port
	if port == 0 {
		port = DefaultListenPort
	}
	return []Check{
		wireGuardCheck(e),
		udpPortCheck(e, port),
		firewallCheck(e, port),
		publicAddressCheck(e),
		{ID: CheckLocalNode, Status: CheckPass, Detail: "this server is registered as the first mesh node when the mesh starts"},
	}
}

func wireGuardCheck(e ReadinessEnv) Check {
	d := Detect(e.Probe)
	switch {
	case d.Backend == BackendDisabled:
		return Check{ID: CheckWireGuard, Status: CheckFail, Detail: d.Reason,
			Hint: "run the control plane as root (or in a container with --cap-add NET_ADMIN --device /dev/net/tun)"}
	case d.Backend == BackendKernel:
		return Check{ID: CheckWireGuard, Status: CheckPass, Detail: d.Reason}
	case e.Probe.OS() == "linux" && !e.TunDevicePresent():
		return Check{ID: CheckWireGuard, Status: CheckFail, Detail: "no in-kernel wireguard module and /dev/net/tun is missing, so wireguard-go cannot create its interface",
			Hint: "load the tun module (sudo modprobe tun) or pass --device /dev/net/tun to the container, or install a kernel with wireguard"}
	}
	return Check{ID: CheckWireGuard, Status: CheckPass, Detail: d.Reason}
}

func udpPortCheck(e ReadinessEnv, port int) Check {
	if e.MeshRunning {
		return Check{ID: CheckUDPPort, Status: CheckPass, Detail: fmt.Sprintf("UDP %d is held by the running mesh", port)}
	}
	free, err := e.UDPPortFree(port)
	switch {
	case err != nil:
		return Check{ID: CheckUDPPort, Status: CheckFail, Detail: fmt.Sprintf("cannot bind UDP %d: %v", port, err),
			Hint: "free the port or run as root"}
	case !free:
		return Check{ID: CheckUDPPort, Status: CheckFail, Detail: fmt.Sprintf("UDP %d is already in use by another process", port),
			Hint: fmt.Sprintf("stop whatever listens on UDP %d (sudo ss -ulpn | grep %d)", port, port)}
	}
	return Check{ID: CheckUDPPort, Status: CheckPass, Detail: fmt.Sprintf("UDP %d is free on this host", port)}
}

func firewallCheck(e ReadinessEnv, port int) Check {
	if e.UFWActive() {
		return Check{ID: CheckFirewall, Status: CheckWarn,
			Detail: fmt.Sprintf("ufw is active: other nodes cannot reach UDP %d unless it is allowed", port),
			Hint:   fmt.Sprintf("sudo ufw allow %d/udp", port)}
	}
	return Check{ID: CheckFirewall, Status: CheckPass,
		Detail: fmt.Sprintf("no host firewall blocking UDP %d was found; a cloud firewall cannot be checked from this server", port)}
}

func publicAddressCheck(e ReadinessEnv) Check {
	h := strings.TrimSpace(e.AdvertiseHost)
	if h == "" || h == "localhost" {
		return unknownAddress()
	}
	if ip, err := netip.ParseAddr(h); err == nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return unknownAddress()
	}
	return Check{ID: CheckPublicAddress, Status: CheckPass, Detail: "other nodes will dial " + h}
}

func unknownAddress() Check {
	return Check{ID: CheckPublicAddress, Status: CheckWarn,
		Detail: "no public address is set for this control plane, so other nodes cannot find it",
		Hint:   "set APP_AGENT_ADVERTISE_HOST to this server's public IP or hostname before adding nodes"}
}

// Blocking returns the checks that failed.
func Blocking(checks []Check) []Check {
	var out []Check
	for _, c := range checks {
		if c.Status == CheckFail {
			out = append(out, c)
		}
	}
	return out
}
