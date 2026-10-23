package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/functions"
)

const fnDevStopTimeout = 15 * time.Second

func runFunctionsDev(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" functions dev", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", functions.DefaultPort, "port the handler serves on, also set as PORT")
	envFile := fs.String("env-file", ".env.local", "file of KEY=VALUE lines passed to the handler, skipped when missing")
	image := fs.String("image", "", "container image to run the handler in, default per runtime")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s functions dev <dir> [--port N] [--env-file FILE] [--image REF]\n\nRuns the handler in its runtime container (Docker Engine API) with PORT and the env file set.\n\nFlags:\n", prog)
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
	env := map[string]string{}
	if data, readErr := os.ReadFile(*envFile); readErr == nil {
		env = functions.ParseEnvFile(data)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		_, _ = fmt.Fprintf(stderr, "%s: read %s: %v\n", prog, *envFile, readErr)
		return exitUsage
	}
	env["PORT"] = strconv.Itoa(*port)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_, _ = fmt.Fprintf(stderr, "running %s handler in %s on http://localhost:%d\n", spec.Runtime, spec.Image, *port)
	if err := runDevContainer(ctx, spec, abs, env, *port, stdout, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitAPIError
	}
	return exitOK
}

func runDevContainer(ctx context.Context, spec functions.DevSpec, dir string, env map[string]string, port int, stdout, stderr io.Writer) error {
	cli, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	id, err := cli.Create(ctx, docker.ContainerSpec{
		Name:       fmt.Sprintf("fn-dev-%d-%d", port, os.Getpid()),
		Image:      spec.Image,
		Command:    spec.ShellCommand(),
		Env:        env,
		Ports:      []docker.PortBinding{{ContainerPort: port, HostPort: port, HostIP: "127.0.0.1"}},
		BindMounts: []docker.BindMount{{HostPath: dir, ContainerPath: functions.DevMountPath}},
	})
	if err != nil {
		return fmt.Errorf("create dev container: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), fnDevStopTimeout)
		defer cancel()
		if rmErr := cli.Remove(cleanup, id, true); rmErr != nil {
			_, _ = fmt.Fprintf(stderr, "remove dev container %s: %v\n", id, rmErr)
		}
	}()
	if err := cli.Start(ctx, id); err != nil {
		return fmt.Errorf("start dev container: %w", err)
	}
	lines, errs := cli.Logs(ctx, id, true, time.Time{})
	for line := range lines {
		w := stdout
		if line.Stream == "stderr" {
			w = stderr
		}
		_, _ = fmt.Fprintln(w, line.Message)
	}
	if err := <-errs; err != nil && ctx.Err() == nil {
		return fmt.Errorf("stream dev container logs: %w", err)
	}
	return nil
}
