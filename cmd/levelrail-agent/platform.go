package main

import "log/slog"

// Logged once so an operator on a Mac sees why Linux-only features are absent instead of finding silent gaps.
func logPlatformLimits(logger *slog.Logger, goos string) {
	if goos != "darwin" {
		return
	}
	logger.Info("not supported on macOS: kernel WireGuard (userspace mesh only, needs root)")
	logger.Info("not supported on macOS: NVIDIA GPU probing (nvidia-smi absent, GPU jobs are never scheduled here)")
	logger.Info("not supported on macOS: /proc host stats (memory, swap and RSS use fallbacks)")
}
