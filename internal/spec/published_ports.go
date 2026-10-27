package spec

import (
	"errors"
	"fmt"
)

// ErrInvalidPublishedPort marks every ValidatePublishedPorts failure.
var ErrInvalidPublishedPort = errors.New("invalid published port")

// Port protocols a PublishedPort may carry.
const (
	PortProtocolTCP = "tcp"
	PortProtocolUDP = "udp"
)

// PublishedPort is one port bound straight on the host beside the routed
// Port, keyed by (HostPort, Protocol): tcp 53 and udp 53 are different.
type PublishedPort struct {
	ContainerPort int    `yaml:"container_port" json:"container_port"`
	HostPort      int    `yaml:"host_port" json:"host_port"`
	Protocol      string `yaml:"protocol,omitempty" json:"protocol"`
}

// EffectiveProtocol is Protocol with an empty value read as tcp.
func (p PublishedPort) EffectiveProtocol() string {
	if p.Protocol == "" {
		return PortProtocolTCP
	}
	return p.Protocol
}

// String renders the port the way operators write it, "51820/udp".
func (p PublishedPort) String() string {
	return fmt.Sprintf("%d/%s", p.HostPort, p.EffectiveProtocol())
}

// ValidatePublishedPorts checks each entry's range and protocol, and that
// no (host port, protocol) pair appears twice.
func ValidatePublishedPorts(ports []PublishedPort) error {
	if err := validatePublishedPorts(ports); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPublishedPort, err)
	}
	return nil
}

func validatePublishedPorts(ports []PublishedPort) error {
	seen := make(map[PublishedPort]bool, len(ports))
	for _, p := range ports {
		proto := p.EffectiveProtocol()
		if proto != PortProtocolTCP && proto != PortProtocolUDP {
			return fmt.Errorf("published port %d: protocol %q is not supported, use tcp or udp", p.HostPort, p.Protocol)
		}
		if p.ContainerPort < 1 || p.ContainerPort > 65535 {
			return fmt.Errorf("published port %s: container port %d must be between 1 and 65535", p, p.ContainerPort)
		}
		if p.HostPort < 1 || p.HostPort > 65535 {
			return fmt.Errorf("published port %d/%s: a host port between 1 and 65535 is required, for example \"%d:%d/%s\"", p.ContainerPort, proto, p.ContainerPort, p.ContainerPort, proto)
		}
		key := PublishedPort{HostPort: p.HostPort, Protocol: proto}
		if seen[key] {
			return fmt.Errorf("published port %s is declared twice", p)
		}
		seen[key] = true
	}
	return nil
}
