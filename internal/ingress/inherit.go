package ingress

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Socket names in the systemd .socket unit (FileDescriptorName=).
const (
	SocketNameHTTPS = "https"
	SocketNameHTTP  = "http"

	// EnvSocketActivation is auto (default), true or false.
	EnvSocketActivation = "APP_INGRESS_SOCKET_ACTIVATION"

	listenFdsStart = 3
)

// InheritedSockets are listener file descriptors handed over by systemd. The
// kernel keeps accepting into their backlog while the control plane restarts,
// so a restart delays connections instead of refusing them.
type InheritedSockets struct {
	HTTPS     int
	HTTP      int
	HTTPSPort int
	HTTPPort  int
}

// Active reports whether the HTTPS listener was inherited.
func (s *InheritedSockets) Active() bool { return s != nil && s.HTTPS != 0 }

// InheritSockets reads the sd_listen_fds protocol. It returns nil with no
// error when nothing was passed. mode is the EnvSocketActivation value: "false"
// ignores inherited sockets, "true" makes their absence an error.
func InheritSockets(getenv func(string) string, pid int, mode string) (*InheritedSockets, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "false" || mode == "0" || mode == "off" {
		return nil, nil
	}
	required := mode == "true" || mode == "1" || mode == "on"
	missing := func(why string) (*InheritedSockets, error) {
		if required {
			return nil, fmt.Errorf("ingress: %s=true but %s", EnvSocketActivation, why)
		}
		return nil, nil
	}

	if getenv("LISTEN_PID") != strconv.Itoa(pid) {
		return missing("this process was not started with systemd socket activation (LISTEN_PID mismatch)")
	}
	n, err := strconv.Atoi(getenv("LISTEN_FDS"))
	if err != nil || n <= 0 {
		return missing("LISTEN_FDS is empty")
	}
	names := strings.Split(getenv("LISTEN_FDNAMES"), ":")
	if len(names) != n {
		return nil, fmt.Errorf("ingress: LISTEN_FDNAMES lists %d names for %d sockets: name them with FileDescriptorName=%s/%s in the .socket unit", len(names), n, SocketNameHTTPS, SocketNameHTTP)
	}
	out := &InheritedSockets{}
	for i, name := range names {
		fd := listenFdsStart + i
		switch name {
		case SocketNameHTTPS:
			out.HTTPS = fd
			out.HTTPSPort = fdTCPPort(fd)
		case SocketNameHTTP:
			out.HTTP = fd
			out.HTTPPort = fdTCPPort(fd)
		default:
			continue
		}
		syscall.CloseOnExec(fd)
	}
	if out.HTTPS == 0 {
		return missing("no socket named " + SocketNameHTTPS + " was passed")
	}
	return out, nil
}

// ClearListenEnv removes the sd_listen_fds variables so child processes do not
// mistake them for their own.
func ClearListenEnv() {
	for _, k := range []string{"LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES"} {
		_ = os.Unsetenv(k)
	}
}

func fdTCPPort(fd int) int {
	// Probe a duplicate: closing an os.File built on fd itself would close the
	// inherited descriptor and let the runtime reuse its number.
	dup, err := syscall.Dup(fd)
	if err != nil {
		return 0
	}
	f := os.NewFile(uintptr(dup), "inherited")
	if f == nil {
		_ = syscall.Close(dup)
		return 0
	}
	ln, err := net.FileListener(f)
	_ = f.Close()
	if err != nil {
		return 0
	}
	defer func() { _ = ln.Close() }()
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

func (s *InheritedSockets) listenAddrs() []string {
	return []string{"fd/" + strconv.Itoa(s.HTTPS)}
}

// redirectServer serves port 80: a plain HTTPS redirect, or nothing when the
// redirect is turned off (the socket stays held so clients are not refused).
func (s *InheritedSockets) redirectServer(redirect bool) *Server {
	srv := &Server{AutomaticHTTPS: &AutoHTTPSConfig{Disabled: true}, Listen: []string{"fd/" + strconv.Itoa(s.HTTP)}}
	if redirect {
		srv.Routes = []Route{{Handle: []any{StaticResponseHandler{
			Handler:    "static_response",
			StatusCode: 308,
			Headers:    map[string][]string{"Location": {"https://{http.request.host}{http.request.uri}"}},
		}}}}
	}
	return srv
}

func withoutProtocol(protocols []string, drop string) []string {
	out := make([]string, 0, len(protocols))
	for _, p := range protocols {
		if p != drop {
			out = append(out, p)
		}
	}
	return out
}

// disableHTTPChallenge turns off ACME HTTP-01 for every policy: an fd listener
// has no port for Caddy to attach the challenge handler to, so TLS-ALPN-01 and
// DNS-01 do the work instead.
func disableHTTPChallenge(app *TLSApp) {
	if app == nil || app.Automation == nil {
		return
	}
	for i := range app.Automation.Policies {
		issuers := app.Automation.Policies[i].Issuers
		for j, iss := range issuers {
			if acme, ok := iss.(ACMEIssuer); ok {
				if acme.Challenges == nil {
					acme.Challenges = &ChallengesConfig{}
				}
				acme.Challenges.HTTP = &HTTPChallengeConfig{Disabled: true}
				issuers[j] = acme
			}
		}
	}
}
