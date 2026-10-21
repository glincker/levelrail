package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
)

const envConnectPassword = "LEVELRAIL_DB_PASSWORD" //nolint:gosec // env var name, not a credential

type connectTarget struct {
	engine   string
	user     string
	database string
	host     string
	port     int
}

// connectString is the connection string for the local end of the tunnel; the password is never part of it.
func connectString(t connectTarget) string {
	addr := net.JoinHostPort(t.host, strconv.Itoa(t.port))
	switch t.engine {
	case "postgres":
		return fmt.Sprintf("postgresql://%s@%s/%s", t.user, addr, t.database)
	case "mysql", "mariadb":
		return fmt.Sprintf("mysql://%s@%s/%s", t.user, addr, t.database)
	case "redis":
		return "redis://" + addr
	}
	return addr
}

// clientCommand maps an engine to its CLI client. The password travels in the environment, never in argv.
func clientCommand(t connectTarget, password string) (bin string, args, env []string, ok bool) {
	port := strconv.Itoa(t.port)
	switch t.engine {
	case "postgres":
		return "psql", []string{"-h", t.host, "-p", port, "-U", t.user, "-d", t.database}, withEnv("PGPASSWORD", password), true
	case "mysql", "mariadb":
		args = []string{"-h", t.host, "-P", port, "-u", t.user}
		if t.database != "" {
			args = append(args, t.database)
		}
		return "mysql", args, withEnv("MYSQL_PWD", password), true
	case "redis":
		return "redis-cli", []string{"-h", t.host, "-p", port}, withEnv("REDISCLI_AUTH", password), true
	}
	return "", nil, nil, false
}

func withEnv(key, value string) []string {
	if value == "" {
		return nil
	}
	return []string{key + "=" + value}
}

func defaultConnectUser(engine string) string {
	switch engine {
	case "postgres":
		return "postgres"
	case "mysql", "mariadb":
		return "root"
	}
	return ""
}

// runDatabasesConnectManaged opens a tunnel to a managed database, prints a connection string and optionally launches the engine's client.
func runDatabasesConnectManaged(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var port int
	var user, database string
	var launch bool
	c, code, ok := parseDBAccess(prog, "databases connect", "databases connect <database> [--user U] [--database DB] [--port N] [--launch] [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&port, "port", 0, "local port to listen on (default: a free port)")
		fs.StringVar(&user, "user", "", "database role to put in the connection string (default: the engine's admin role)")
		fs.StringVar(&database, "database", "", "database name inside the server (default: the engine's default)")
		fs.BoolVar(&launch, "launch", false, "start psql, mysql or redis-cli against the tunnel; the password is read from env "+envConnectPassword)
	})
	if !ok {
		return code
	}
	name := c.rest[0]
	res, err := c.client.GetDatabase(context.Background(), name)
	if err != nil {
		return c.fail(fmt.Errorf("get database %q: %w", name, err))
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return c.fail(fmt.Errorf("listen on 127.0.0.1:%d: %w", port, err))
	}
	defer func() { _ = ln.Close() }()

	if user == "" {
		user = defaultConnectUser(res.Engine)
	}
	if database == "" && res.Engine == "postgres" {
		database = "postgres"
	}
	target := connectTarget{engine: res.Engine, user: user, database: database, host: "127.0.0.1", port: ln.Addr().(*net.TCPAddr).Port}
	_, _ = fmt.Fprintf(stdout, "%s\n", connectString(target))
	_, _ = fmt.Fprintf(stderr, "tunnel to database %q on %s (Ctrl+C to stop); the password is not shown, see 'databases users' to issue one\n", name, ln.Addr())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveTunnel(ctx, ln, func(dctx context.Context) (net.Conn, error) {
			return c.client.DialDatabaseTunnel(dctx, name)
		}, stderr)
	}()

	if launch {
		pw, _ := lookupEnv(envConnectPassword)
		bin, cargs, env, known := clientCommand(target, pw)
		path, lookErr := exec.LookPath(bin)
		switch {
		case !known:
			_, _ = fmt.Fprintf(stderr, "no command line client for engine %q; tunnel stays open\n", res.Engine)
		case lookErr != nil:
			_, _ = fmt.Fprintf(stderr, "%s is not installed; tunnel stays open, connect with the string above\n", bin)
		default:
			cmd := exec.CommandContext(ctx, path, cargs...) //nolint:gosec // fixed client binary chosen by engine, argv form
			cmd.Env = append(os.Environ(), env...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			if runErr := cmd.Run(); runErr != nil && ctx.Err() == nil {
				_, _ = fmt.Fprintf(stderr, "%s exited: %v\n", bin, runErr)
			}
			stop()
		}
	}
	<-done
	return exitOK
}
