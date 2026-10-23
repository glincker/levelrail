package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/diskspace"
)

const (
	envDockerHost          = "DOCKER_HOST"
	envDataDir             = "APP_DATA_DIR"
	envDoctorDiskWarnBytes = "APP_DOCTOR_DISK_WARNING_BYTES"

	defaultDockerSocket  = "/var/run/docker.sock"
	localDockerTimeout   = 3 * time.Second
	localDiskWarnBytes   = int64(1) << 30
	localDoctorGroupHost = "host"
)

// localDoctorReport is the offline subset: Docker reachability, disk space and the web ports.
func localDoctorReport(ctx context.Context, lookupEnv func(string) (string, bool)) systemDoctorResource {
	checks := []doctorCheckResource{
		localDockerCheck(ctx, lookupEnv),
		localDiskCheck(lookupEnv),
		localPortCheck(ctx, 80),
		localPortCheck(ctx, 443),
	}
	ok := true
	for _, c := range checks {
		if c.Status == "fail" {
			ok = false
		}
	}
	return systemDoctorResource{OK: ok, Checks: checks}
}

func dockerSocketPath(lookupEnv func(string) (string, bool)) string {
	if v, ok := lookupEnv(envDockerHost); ok && strings.HasPrefix(v, "unix://") {
		return strings.TrimPrefix(v, "unix://")
	}
	return defaultDockerSocket
}

func localDockerCheck(ctx context.Context, lookupEnv func(string) (string, bool)) doctorCheckResource {
	c := doctorCheckResource{Code: "docker", Name: "Docker daemon", Group: localDoctorGroupHost}
	sock := dockerSocketPath(lookupEnv)
	client := &http.Client{
		Timeout: localDockerTimeout,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		}},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/version", nil)
	if err != nil {
		c.Status, c.Message = "unknown", err.Error()
		return c
	}
	resp, err := client.Do(req)
	if err != nil {
		c.Status, c.Message, c.Evidence = "fail", "cannot reach the Docker daemon", err.Error()
		c.Fix = "start Docker (systemctl start docker) and make sure this user can read " + sock
		return c
	}
	_ = resp.Body.Close()
	c.Status, c.Message = "ok", "reachable at "+sock
	return c
}

func localDiskCheck(lookupEnv func(string) (string, bool)) doctorCheckResource {
	c := doctorCheckResource{Code: "disk_space", Name: "Disk space", Group: localDoctorGroupHost}
	dir := "/"
	if v, ok := lookupEnv(envDataDir); ok && v != "" {
		dir = v
	}
	free, err := diskspace.Free(dir)
	if err != nil {
		c.Status, c.Message = "unknown", fmt.Sprintf("could not read disk usage of %s: %s", dir, err)
		return c
	}
	warn := localDiskWarnBytes
	if v, ok := lookupEnv(envDoctorDiskWarnBytes); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			warn = n
		}
	}
	c.Evidence = fmt.Sprintf("%s free on the volume holding %s", diskspace.HumanBytes(free), dir)
	if free < warn {
		c.Status = "warn"
		c.Message = fmt.Sprintf("%s free, below the %s warning threshold", diskspace.HumanBytes(free), diskspace.HumanBytes(warn))
		c.Fix = "free disk space, for example docker system prune after checking what it would remove"
		return c
	}
	c.Status, c.Message = "ok", diskspace.HumanBytes(free)+" free"
	return c
}

func localPortCheck(ctx context.Context, port int) doctorCheckResource {
	c := doctorCheckResource{Code: fmt.Sprintf("port_%d", port), Name: fmt.Sprintf("Port %d available", port), Group: "network"}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	switch {
	case err == nil:
		_ = ln.Close()
		c.Status, c.Message = "ok", "available"
	case errors.Is(err, syscall.EADDRINUSE):
		c.Status, c.Message = "warn", "in use, expected if the control plane or another web server runs on this host"
		c.Fix = "if the control plane is not running here, find the holder with: ss -ltnp 'sport = :" + strconv.Itoa(port) + "'"
	case errors.Is(err, os.ErrPermission):
		c.Status, c.Message = "unknown", "cannot check without elevated privileges"
	default:
		c.Status, c.Message = "unknown", err.Error()
	}
	return c
}
