package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Letters only so the mask survives URL escaping unchanged.
const connectionStringMask = "xxxxxxxx"

type connStringTarget struct {
	engine   string
	user     string
	password string
	host     string
	port     int
	database string
	tls      bool
}

func defaultEnginePort(engine string) int {
	switch engine {
	case "postgres":
		return 5432
	case "mysql", "mariadb":
		return 3306
	case "redis", "valkey", "keydb":
		return 6379
	case "mongodb":
		return 27017
	}
	return 0
}

// buildConnectionURL renders the engine URL; url.URL escapes the password so special characters stay valid.
func buildConnectionURL(t connStringTarget) (string, bool) {
	u := url.URL{Host: net.JoinHostPort(t.host, strconv.Itoa(t.port))}
	switch t.engine {
	case "postgres":
		u.Scheme = "postgres"
		u.Path = "/" + t.database
		if t.tls {
			u.RawQuery = "sslmode=require"
		} else {
			u.RawQuery = "sslmode=disable"
		}
	case "mysql", "mariadb":
		u.Scheme = "mysql"
		u.Path = "/" + t.database
	case "redis", "valkey", "keydb":
		u.Scheme = "redis"
		if t.tls {
			u.Scheme = "rediss"
		}
	case "mongodb":
		u.Scheme = "mongodb"
		u.Path = "/" + t.database
	default:
		return "", false
	}
	if t.engine == "redis" || t.engine == "valkey" || t.engine == "keydb" {
		if t.password != "" {
			u.User = url.UserPassword(t.user, t.password)
		}
	} else if t.user != "" {
		u.User = url.UserPassword(t.user, t.password)
	}
	return u.String(), true
}

func renderConnectionString(t connStringTarget, format string) (string, error) {
	connURL, ok := buildConnectionURL(t)
	if !ok {
		return "", fmt.Errorf("engine %q has no connection URL form", t.engine)
	}
	switch format {
	case "url":
		return connURL, nil
	case "env":
		return "DATABASE_URL=" + connURL, nil
	case "json":
		parts := map[string]string{"url": connURL, "engine": t.engine, "host": t.host, "port": strconv.Itoa(t.port), "user": t.user, "password": t.password, "database": t.database}
		keys := make([]string, 0, len(parts))
		for k := range parts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			_, _ = fmt.Fprintf(&b, "%q:%q", k, parts[k])
		}
		b.WriteString("}")
		return b.String(), nil
	}
	return "", fmt.Errorf("unknown --format %q (want url, env or json)", format)
}

func runDatabasesConnectionString(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var format, user, database, host string
	var port int
	var reveal bool
	usage := "databases connection-string <database> [--format url|env|json] [--reveal] [--host H] [--port N] [--user U] [--database DB] [flags]\n\n" +
		"Prints a ready-made connection URL. The password is masked unless --reveal, which\n" +
		"is owner only, asks for step-up and is audited like 'databases credentials'."
	c, code, ok := parseDBAccess(prog, "databases connection-string", usage, args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&format, "format", "url", "output format: url, env or json")
		fs.BoolVar(&reveal, "reveal", false, "include the real owner password (step-up, audited)")
		fs.StringVar(&host, "host", "", "host to put in the URL (default: the database name, reachable inside the platform network)")
		fs.IntVar(&port, "port", 0, "port (default: the public port if public, else the engine default)")
		fs.StringVar(&user, "user", "", "role (default: the engine's admin role)")
		fs.StringVar(&database, "database", "", "database name inside the server (default: the engine's default)")
	})
	if !ok {
		return code
	}
	name := strings.TrimSpace(c.rest[0])
	res, err := c.client.GetDatabase(context.Background(), name)
	if err != nil {
		return c.fail(fmt.Errorf("get database %q: %w", name, err))
	}
	t := connStringTarget{engine: res.Engine, user: user, password: connectionStringMask, host: host, port: port, database: database, tls: res.TLSEnabled}
	if t.host == "" {
		t.host = name
	}
	if t.port == 0 {
		t.port = defaultEnginePort(res.Engine)
		if res.PubliclyAccessible && res.PublicPort > 0 {
			t.port = res.PublicPort
		}
	}
	if t.user == "" {
		t.user = defaultConnectUser(res.Engine)
		if res.Engine == "mongodb" {
			t.user = "root"
		}
	}
	if t.database == "" {
		switch res.Engine {
		case "postgres":
			t.database = "postgres"
		case "mongodb":
			t.database = "admin"
		}
	}
	if reveal {
		cred, rerr := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.DatabaseRevealedCredential, error) {
			return c.client.RevealDatabaseCredentials(context.Background(), name, p)
		})
		if rerr != nil {
			return c.fail(fmt.Errorf("reveal credentials of database %q: %w", name, rerr))
		}
		t.password = cred.Password
	}
	out, err := renderConnectionString(t, format)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}
	_, _ = fmt.Fprintln(stdout, out)
	return exitOK
}
