// Package imagemove streams a host-built image from a source Docker host
// over SSH into a target node's image store and verifies it by image ID.
package imagemove

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/distribution/reference"
)

// DefaultSSHPort is used when the target names no port.
const DefaultSSHPort = 22

const maxImageRefLen = 255

var (
	// No shell metacharacter, space or quote can pass: the ref ends up in the remote command line.
	imageRefRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]*$`)
	imageIDRe  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	sshUserRe  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,31}$`)
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
)

// Target is a validated SSH login on the source host.
type Target struct {
	User string `json:"user"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Addr is the host:port to dial.
func (t Target) Addr() string { return net.JoinHostPort(t.Host, strconv.Itoa(t.Port)) }

// String renders the target as user@host:port.
func (t Target) String() string { return t.User + "@" + t.Addr() }

// ValidateImageRef accepts a plain repository reference with an optional
// tag. Digests, uppercase names and anything that could act as a remote
// shell token or a command line option are refused.
func ValidateImageRef(ref string) error {
	if ref == "" || len(ref) > maxImageRefLen {
		return errors.New("image reference must be 1 to 255 characters")
	}
	if !imageRefRe.MatchString(ref) || strings.Contains(ref, "..") || strings.Contains(ref, "//") {
		return fmt.Errorf("image reference %q has characters that are not allowed", ref)
	}
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return fmt.Errorf("image reference %q is not valid: %w", ref, err)
	}
	if _, ok := named.(reference.Digested); ok {
		return fmt.Errorf("image reference %q must not carry a digest", ref)
	}
	return nil
}

// ValidateImageID accepts a full sha256 image ID as `docker inspect` prints it.
func ValidateImageID(id string) error {
	if !imageIDRe.MatchString(id) {
		return fmt.Errorf("image ID %q is not a full sha256 ID", id)
	}
	return nil
}

// ParseTarget parses user@host, user@host:port or user@[ipv6]:port. A
// non-zero port argument is used when the string names none, and must
// agree with it when it does.
func ParseTarget(s string, port int) (Target, error) {
	user, rest, ok := strings.Cut(strings.TrimSpace(s), "@")
	if !ok || user == "" || rest == "" {
		return Target{}, errors.New("ssh target must look like user@host")
	}
	if !sshUserRe.MatchString(user) {
		return Target{}, fmt.Errorf("ssh user %q is not allowed", user)
	}
	host, inPort, err := splitHostPort(rest)
	if err != nil {
		return Target{}, err
	}
	switch {
	case inPort != 0 && port != 0 && inPort != port:
		return Target{}, fmt.Errorf("ssh port %d conflicts with %d in the target", port, inPort)
	case inPort == 0 && port != 0:
		inPort = port
	case inPort == 0:
		inPort = DefaultSSHPort
	}
	if inPort < 1 || inPort > 65535 {
		return Target{}, fmt.Errorf("ssh port %d is out of range", inPort)
	}
	return Target{User: user, Host: host, Port: inPort}, nil
}

func splitHostPort(s string) (string, int, error) {
	host, portStr := s, ""
	switch {
	case strings.HasPrefix(s, "["):
		end := strings.Index(s, "]")
		if end < 0 {
			return "", 0, errors.New("ssh host has an unclosed [")
		}
		host, portStr = s[1:end], s[end+1:]
		if portStr != "" && !strings.HasPrefix(portStr, ":") {
			return "", 0, errors.New("ssh host has text after ]")
		}
		portStr = strings.TrimPrefix(portStr, ":")
		if ip := net.ParseIP(host); ip == nil || ip.To4() != nil {
			return "", 0, fmt.Errorf("ssh host %q is not an IPv6 address", host)
		}
	case strings.Count(s, ":") == 1:
		host, portStr, _ = strings.Cut(s, ":")
	}
	if net.ParseIP(host) == nil && (len(host) > 253 || !hostnameRe.MatchString(host)) {
		return "", 0, fmt.Errorf("ssh host %q is not a hostname or IP address", host)
	}
	if portStr == "" {
		return host, 0, nil
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p < 1 || p > 65535 {
		return "", 0, fmt.Errorf("ssh port %q is not a number from 1 to 65535", portStr)
	}
	return host, p, nil
}

// SaveCommand is the only command ever run on the source host.
func SaveCommand(ref string) (string, error) {
	if err := ValidateImageRef(ref); err != nil {
		return "", err
	}
	return "docker save " + ref, nil
}
