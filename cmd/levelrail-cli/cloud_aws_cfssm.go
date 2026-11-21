package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runCloudAWSCFSSM handles the cloudfront and ssm verbs. Both stay off until their runtime flags are on.
func runCloudAWSCFSSM(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var region, instance, command string
	var paths multiFlag
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&region, "region", "", "AWS region, defaults to the account region")
		fs.StringVar(&instance, "instance", "", "instance ID, for ssm")
		fs.StringVar(&command, "command", "", "allowlisted shell command, for ssm run")
		fs.Var(&paths, "path", "path to invalidate, repeatable, for cloudfront invalidate")
	}
	return cloudCall(prog, "cloud aws "+verb, args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
		if len(pos) < 2 {
			return nil, nil, fmt.Errorf("a subcommand and an account ID are required (see help)")
		}
		sub, acct, rest := pos[0], pos[1], pos[2:]
		base := "/" + url.PathEscape(acct) + "/aws/"
		q := url.Values{}
		if region != "" {
			q.Set("region", region)
		}
		withQ := func(p string) string {
			if len(q) == 0 {
				return p
			}
			return p + "?" + q.Encode()
		}
		ctx := context.Background()
		var out cloudRow
		call := func(method, p string, body any) error { return c.CloudRequest(ctx, method, p, body, &out) }
		table := func(head []string, key string, keys ...string) func() {
			return func() { printCloudTable(stdout, head, rowsOf(out[key]), keys...) }
		}
		needID := func(what string) (string, error) {
			if len(rest) < 1 {
				return "", fmt.Errorf("%s is required", what)
			}
			return url.PathEscape(rest[0]), nil
		}
		var err error
		switch verb + " " + sub {
		case "cloudfront list":
			err = call("GET", base+"cloudfront/distributions", nil)
			return out, table([]string{"ID", "DOMAIN", "STATUS", "ENABLED", "COMMENT"}, "distributions", "id", "domain", "status", "enabled", "comment"), err
		case "cloudfront show":
			id, e := needID("a distribution ID")
			if e != nil {
				return nil, nil, e
			}
			err = call("GET", base+"cloudfront/distributions/"+id, nil)
			return out, func() {
				printCloudTable(stdout, []string{"ID", "DOMAIN", "STATUS"}, []cloudRow{out}, "id", "domain", "status")
			}, err
		case "cloudfront invalidations":
			id, e := needID("a distribution ID")
			if e != nil {
				return nil, nil, e
			}
			err = call("GET", base+"cloudfront/distributions/"+id+"/invalidations", nil)
			return out, table([]string{"ID", "STATUS", "CREATED"}, "invalidations", "id", "status", "created_at"), err
		case "cloudfront invalidate":
			id, e := needID("a distribution ID")
			if e != nil {
				return nil, nil, e
			}
			if len(paths) == 0 {
				return nil, nil, fmt.Errorf("at least one --path is required")
			}
			err = call("POST", base+"cloudfront/distributions/"+id+"/invalidations", cloudRow{"paths": []string(paths)})
			return out, func() { printCloudTable(stdout, []string{"ID", "STATUS"}, []cloudRow{out}, "id", "status") }, err
		case "ssm instances":
			if instance != "" {
				q.Set("instance", instance)
			}
			err = call("GET", withQ(base+"ssm/instances"), nil)
			return out, table([]string{"ID", "PING", "PLATFORM", "AGENT", "LAST PING"}, "instances", "id", "ping_status", "platform_name", "agent_version", "last_ping"), err
		case "ssm run":
			if instance == "" || strings.TrimSpace(command) == "" {
				return nil, nil, fmt.Errorf("--instance and --command are required")
			}
			err = call("POST", base+"ssm/commands", cloudRow{"instance": instance, "command": command, "region": region})
			return out, func() { _, _ = fmt.Fprintln(stdout, out["command_id"]) }, err
		case "ssm result":
			id, e := needID("a command ID")
			if e != nil {
				return nil, nil, e
			}
			if instance == "" {
				return nil, nil, fmt.Errorf("--instance is required")
			}
			err = call("POST", base+"ssm/commands/"+id+"/invocation", cloudRow{"instance": instance, "region": region})
			return out, func() {
				_, _ = fmt.Fprintf(stderr, "status %v exit %v\n", out["status"], out["exit_code"])
				_, _ = fmt.Fprint(stdout, out["stdout"])
			}, err
		case "ssm session":
			if instance == "" {
				return nil, nil, fmt.Errorf("--instance is required")
			}
			err = call("POST", base+"ssm/sessions", cloudRow{"instance": instance, "region": region})
			return out, func() {
				_, _ = fmt.Fprintf(stdout, "session %v\nstream %v\ntoken %v\n", out["session_id"], out["stream_url"], out["token"])
			}, err
		}
		return nil, nil, fmt.Errorf("unknown subcommand %q", sub)
	})
}
