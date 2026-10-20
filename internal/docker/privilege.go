package docker

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"slices"
	"strconv"
	"strings"
)

// DaemonSecurity is what the daemon itself says about its isolation.
type DaemonSecurity struct {
	Rootless bool
	// UsernsRemap means container root maps to an unprivileged host user.
	UsernsRemap bool
	Options     []string
}

// DaemonSecurity reads the daemon's SecurityOptions from /info.
func (c *Client) DaemonSecurity(ctx context.Context) (DaemonSecurity, error) {
	info, err := c.cli.Info(ctx)
	if err != nil {
		return DaemonSecurity{}, fmt.Errorf("docker: info: %w", err)
	}
	return ParseSecurityOptions(info.SecurityOptions), nil
}

// ParseSecurityOptions reads entries such as "name=rootless" and
// "name=userns" from the daemon's SecurityOptions.
func ParseSecurityOptions(opts []string) DaemonSecurity {
	s := DaemonSecurity{Options: opts}
	for _, o := range opts {
		for _, kv := range strings.Split(o, ",") {
			switch strings.TrimSpace(kv) {
			case "name=rootless":
				s.Rootless = true
			case "name=userns":
				s.UsernsRemap = true
			}
		}
	}
	return s
}

// ServiceUser describes how this process can reach the daemon socket.
type ServiceUser struct {
	Name        string
	Root        bool
	DockerGroup bool
}

// CurrentServiceUser reports whether this process runs as root or holds the
// docker group, either of which makes the socket root equivalent.
func CurrentServiceUser() ServiceUser {
	su := ServiceUser{Root: os.Geteuid() == 0}
	if u, err := user.Current(); err == nil {
		su.Name = u.Username
	}
	g, err := user.LookupGroup("docker")
	if err != nil {
		return su
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return su
	}
	groups, err := os.Getgroups()
	su.DockerGroup = err == nil && (slices.Contains(groups, gid) || os.Getegid() == gid)
	return su
}
