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

// cloudAWSValueEnv carries a secret value so it never lands in shell history or the process list.
const cloudAWSValueEnv = "LEVELRAIL_AWS_SECRET_VALUE" //nolint:gosec // env var name, not a credential

func cloudAWSServicesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cloud aws ecr list|login ACCOUNT [--region R]
  %[1]s cloud aws ecr create ACCOUNT NAME [--region R] [--env E] [--project P]
  %[1]s cloud aws acm list ACCOUNT [--region R]
  %[1]s cloud aws acm request ACCOUNT DOMAIN [--alt NAME]... [--region R]
  %[1]s cloud aws secrets list ACCOUNT [--path /P] [--region R]
  %[1]s cloud aws secrets put ACCOUNT /NAME [--overwrite] [--region R]
  %[1]s cloud aws secrets reveal ACCOUNT NAME [--kind ssm|secretsmanager] [--region R]
  %[1]s cloud aws rds list|show|snapshot|stop|start|adopt|connect ACCOUNT [ID] [--region R]
  %[1]s cloud aws rds create ACCOUNT ID --engine E --class C [--username U] [--storage GB] [--multi-az] [--cluster] [--confirm] [--env E] [--project P]
  %[1]s cloud aws audit ACCOUNT RESOURCE [--region R]
  %[1]s cloud aws guard ACCOUNT [--kill-switch on|off]
  %[1]s cloud aws cleanup-plan ACCOUNT [--region R]

Every command takes --json. Every service is off until enabled in runtime settings. "secrets put" reads the value from %[2]s.
"cleanup-plan" is a dry run and never deletes anything.
`, prog, cloudAWSValueEnv)
}

func isCloudAWSService(v string) bool {
	switch v {
	case "ecr", "acm", "secrets", "rds", "audit", "guard", "cleanup-plan":
		return true
	}
	return false
}

func runCloudAWSServices(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var region, env, project, path, kind, killSwitch, engine, class, username, dbName string
	var storage int
	var overwrite, multiAZ, cluster, confirm bool
	var alts multiFlag
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&region, "region", "", "AWS region, defaults to the account region")
		fs.StringVar(&env, "env", "", "env tag for new resources")
		fs.StringVar(&project, "project", "", "project tag for new resources")
		fs.StringVar(&path, "path", "", "SSM path prefix")
		fs.StringVar(&kind, "kind", "ssm", "ssm or secretsmanager")
		fs.StringVar(&killSwitch, "kill-switch", "", "on or off, root only")
		fs.BoolVar(&overwrite, "overwrite", false, "overwrite an existing parameter")
		fs.StringVar(&engine, "engine", "", "database engine, for rds create")
		fs.StringVar(&class, "class", "", "instance class, for rds create")
		fs.StringVar(&username, "username", "", "master username, for rds create")
		fs.StringVar(&dbName, "db-name", "", "initial database name, for rds create")
		fs.IntVar(&storage, "storage", 20, "allocated storage in GB, for rds create")
		fs.BoolVar(&multiAZ, "multi-az", false, "Multi-AZ deployment, needs --confirm")
		fs.BoolVar(&cluster, "cluster", false, "create an Aurora cluster, needs --confirm")
		fs.BoolVar(&confirm, "confirm", false, "confirm a Multi-AZ or Aurora create")
		fs.Var(&alts, "alt", "alternative certificate name, repeatable")
	}
	label := "cloud aws " + verb
	return cloudCall(prog, label, args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
		sub := ""
		if verb == "ecr" || verb == "acm" || verb == "secrets" || verb == "rds" {
			if len(pos) < 1 {
				return nil, nil, fmt.Errorf("a subcommand is required (see help)")
			}
			sub, pos = pos[0], pos[1:]
		}
		if len(pos) < 1 {
			return nil, nil, fmt.Errorf("an account ID is required")
		}
		rest := pos[1:]
		base := "/" + url.PathEscape(pos[0]) + "/aws/"
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
		if verb == "rds" {
			switch sub {
			case "list", "show", "snapshot", "stop", "start", "adopt", "connect", "create":
			default:
				return nil, nil, fmt.Errorf("unknown subcommand %q", sub)
			}
		}
		var err error
		switch verb + " " + sub {
		case "ecr list":
			err = call("GET", withQ(base+"ecr/repositories"), nil)
			return out, table([]string{"NAME", "URI", "SCAN"}, "repositories", "name", "uri", "scan_on_push"), err
		case "ecr create":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a repository name is required")
			}
			err = call("POST", base+"ecr/repositories", cloudRow{"name": rest[0], "region": region, "env": env, "project": project})
			return out, func() { printCloudTable(stdout, []string{"NAME", "URI"}, []cloudRow{out}, "name", "uri") }, err
		case "ecr login":
			err = call("POST", withQ(base+"ecr/login-token"), nil)
			return out, func() {
				_, _ = fmt.Fprintf(stderr, "Pipe stdout into: docker login --username %v --password-stdin %v (expires %v)\n", out["user"], out["registry"], out["expires_at"])
				_, _ = fmt.Fprintln(stdout, out["password"])
			}, err
		case "acm list":
			err = call("GET", withQ(base+"acm/certificates"), nil)
			return out, table([]string{"DOMAIN", "STATUS", "ARN"}, "certificates", "domain", "status", "arn"), err
		case "acm request":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a domain is required")
			}
			err = call("POST", base+"acm/certificates", cloudRow{"domain": rest[0], "alt_names": []string(alts), "region": region, "env": env, "project": project})
			return out, table([]string{"NAME", "TYPE", "VALUE"}, "records", "name", "type", "value"), err
		case "secrets list":
			if path != "" {
				q.Set("path", path)
			}
			err = call("GET", withQ(base+"secrets"), nil)
			return out, func() {
				printCloudTable(stdout, []string{"SSM NAME", "TYPE", "VALUE"}, rowsOf(out["ssm_parameters"]), "name", "type", "value")
				printCloudTable(stdout, []string{"SECRET", "ARN", "VALUE"}, rowsOf(out["secrets_manager"]), "name", "arn", "value")
			}, err
		case "secrets put":
			value, _ := lookupEnv(cloudAWSValueEnv)
			if len(rest) < 1 || value == "" {
				return nil, nil, fmt.Errorf("a parameter name and %s are required", cloudAWSValueEnv)
			}
			err = call("POST", base+"secrets", cloudRow{"name": rest[0], "value": value, "region": region, "overwrite": overwrite})
			return out, func() { printCloudTable(stdout, []string{"NAME", "VERSION"}, []cloudRow{out}, "name", "version") }, err
		case "secrets reveal":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a name is required")
			}
			err = call("POST", base+"secrets/reveal", cloudRow{"kind": kind, "name": rest[0], "region": region})
			return out, func() { _, _ = fmt.Fprintln(stdout, out["value"]) }, err
		case "rds list":
			err = call("GET", withQ(base+"rds/instances"), nil)
			return out, table([]string{"ID", "ENGINE", "CLASS", "STATUS", "MANAGED", "EST USD/MO"}, "instances", "id", "engine", "class", "status", "managed", "est_monthly_usd"), err
		case "rds show":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("an instance ID is required")
			}
			err = call("GET", withQ(base+"rds/instances/"+url.PathEscape(rest[0])), nil)
			return out, func() {
				printCloudTable(stdout, []string{"ID", "ENGINE", "CLASS", "STATUS", "HOST", "PORT", "MANAGED"}, []cloudRow{out}, "id", "engine", "class", "status", "host", "port", "managed")
			}, err
		case "rds snapshot", "rds stop", "rds start", "rds adopt":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("an instance ID is required")
			}
			err = call("POST", base+"rds/instances/"+url.PathEscape(rest[0])+"/actions", cloudRow{"action": sub, "region": region, "env": env, "project": project})
			return out, func() {
				printCloudTable(stdout, []string{"ID", "ACTION", "SNAPSHOT"}, []cloudRow{out}, "id", "action", "snapshot_id")
			}, err
		case "rds connect":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("an instance ID is required")
			}
			err = call("POST", base+"rds/instances/"+url.PathEscape(rest[0])+"/connect-as-database", cloudRow{"region": region})
			return out, func() {
				printCloudTable(stdout, []string{"NAME", "ENGINE", "HOST", "PORT"}, []cloudRow{out}, "name", "engine", "host", "port")
			}, err
		case "rds create":
			if len(rest) < 1 || engine == "" {
				return nil, nil, fmt.Errorf("an instance ID and --engine are required")
			}
			err = call("POST", base+"rds/instances", cloudRow{"id": rest[0], "engine": engine, "class": class, "master_username": username, "db_name": dbName,
				"allocated_gb": storage, "multi_az": multiAZ, "cluster": cluster, "confirm": confirm, "region": region, "env": env, "project": project})
			return out, func() {
				printCloudTable(stdout, []string{"ID", "EST USD/MO"}, []cloudRow{out}, "id", "est_monthly_usd")
			}, err
		case "audit ":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a resource name is required")
			}
			q.Set("resource", rest[0])
			err = call("GET", withQ(base+"audit"), nil)
			return out, table([]string{"TIME", "EVENT", "USER", "SOURCE"}, "events", "time", "name", "username", "source"), err
		case "guard ":
			if killSwitch != "" {
				if killSwitch != "on" && killSwitch != "off" {
					return nil, nil, fmt.Errorf("--kill-switch takes on or off")
				}
				err = call("POST", base+"guard/kill-switch", cloudRow{"enabled": killSwitch == "on"})
				return out, func() { printCloudTable(stdout, []string{"KILL SWITCH"}, []cloudRow{out}, "kill_switch") }, err
			}
			err = call("GET", base+"guard", nil)
			return out, func() {
				printCloudTable(stdout, []string{"KILL SWITCH", "MONTHLY CAP", "PER CREATE MAX", "MTD SPEND", "SPEND"}, []cloudRow{out}, "kill_switch", "monthly_cap_usd", "per_create_max_usd", "mtd_spend_usd", "spend_state")
			}, err
		case "cleanup-plan ":
			err = call("GET", withQ(base+"cleanup/plan"), nil)
			return out, func() {
				printCloudTable(stdout, []string{"EXPIRED ARN", "EXPIRED AT"}, rowsOf(out["expired"]), "arn", "expired_at")
				if o, ok := out["orphans"].(map[string]any); ok {
					for _, k := range []string{"volumes", "addresses", "snapshots"} {
						printCloudTable(stdout, []string{strings.ToUpper(k), "DETAIL"}, rowsOf(o[k]), "id", "detail")
					}
				}
			}, err
		}
		return nil, nil, fmt.Errorf("unknown subcommand %q", sub)
	})
}

func rowsOf(v any) []cloudRow {
	list, _ := v.([]any)
	rows := make([]cloudRow, 0, len(list))
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }
