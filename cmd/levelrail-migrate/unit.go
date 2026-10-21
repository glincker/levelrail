package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const unitDir = "/etc/systemd/system"

func unitPath() string { return filepath.Join(unitDir, exeName()+".service") }

// renderUnit builds a hardened unit. The token is never written to it: enrollment
// happens in install and the unit only carries the non-secret control plane address.
func renderUnit(exe, addr, stateDir string) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=Read-only migration agent",
		"After=network-online.target docker.service",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"ExecStart=" + exe + " run",
		"Environment=APP_CONTROL_PLANE_ADDR=" + addr,
		"Environment=APP_MIGRATE_STATE_DIR=" + stateDir,
		"Restart=on-failure",
		"RestartSec=5",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
		"ProtectHome=read-only",
		"PrivateTmp=yes",
		"ReadWritePaths=" + stateDir,
		"CapabilityBoundingSet=CAP_DAC_READ_SEARCH",
		"AmbientCapabilities=CAP_DAC_READ_SEARCH",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}

func systemctl(ctx context.Context, args ...string) error {
	out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput() //nolint:gosec // fixed binary, arguments are built in this file
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func install(ctx context.Context, cfg settings, logger *slog.Logger) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find own path: %w", err)
	}
	if _, err := cfg.enrollAndSave(ctx, logger); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath(), []byte(renderUnit(exe, cfg.addr, cfg.stateDir)), 0o644); err != nil { //nolint:gosec // systemd unit files are world readable by convention
		return fmt.Errorf("write unit: %w", err)
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := systemctl(ctx, "enable", "--now", exeName()+".service"); err != nil {
		return err
	}
	logger.Info("installed and started", slog.String("unit", unitPath()))
	return nil
}

// remove undoes install completely. Each step is attempted even if an earlier one fails.
func remove(ctx context.Context, cfg settings, logger *slog.Logger) error {
	var errs []error
	step := func(name string, err error) {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("remove step failed", slog.String("step", name), slog.String("error", err.Error()))
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			return
		}
		logger.Info("removed", slog.String("step", name))
	}
	unit := exeName() + ".service"
	if _, err := exec.LookPath("systemctl"); err == nil {
		step("stop unit", systemctl(ctx, "disable", "--now", unit))
	}
	step("unit file", os.Remove(unitPath()))
	if _, err := exec.LookPath("systemctl"); err == nil {
		step("reload systemd", systemctl(ctx, "daemon-reload"))
	}
	step("identity and state", os.RemoveAll(cfg.stateDir))
	exe, err := os.Executable()
	if err == nil {
		step("binary", os.Remove(exe))
	}
	logger.Info("the control plane session is revoked separately; run the revoke command there if this host was unreachable")
	return errors.Join(errs...)
}
