package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/functions"
)

func runFunctionsDev(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" functions dev", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", functions.DefaultPort, "port the handler serves on, also set as PORT")
	envFile := fs.String("env-file", ".env.local", "file of KEY=VALUE lines passed to the handler, skipped when missing")
	image := fs.String("image", "", "container image to run the handler in, default per runtime")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s functions dev <dir> [--port N] [--env-file FILE] [--image REF]\n\nRuns the handler in its runtime container with PORT and the env file set.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitUsage
	}
	abs, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: resolve %s: %v\n", prog, fs.Arg(0), err)
		return exitUsage
	}
	spec, err := functions.DetectDevSpec(abs, *image)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}
	envPath := ""
	if _, statErr := os.Stat(*envFile); statErr == nil {
		if envPath, err = filepath.Abs(*envFile); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: resolve %s: %v\n", prog, *envFile, err)
			return exitUsage
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_, _ = fmt.Fprintf(stderr, "running %s handler in %s on http://localhost:%d\n", spec.Runtime, spec.Image, *port)
	cmd := exec.CommandContext(ctx, "docker", spec.DockerRunArgs(abs, envPath, *port)...) //nolint:gosec // G204: arguments are built from flags the operator typed
	cmd.Stdout, cmd.Stderr, cmd.Stdin = stdout, stderr, os.Stdin
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		_, _ = fmt.Fprintf(stderr, "%s: docker run: %v\n", prog, err)
		return exitAPIError
	}
	return exitOK
}
