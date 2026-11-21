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
  %[1]s cloud aws rds clusters|cluster-snapshots ACCOUNT [--region R]
  %[1]s cloud aws rds cluster-snapshot|cluster-stop|cluster-start ACCOUNT CLUSTER [--region R]
  %[1]s cloud aws rds cluster-snapshot-delete ACCOUNT SNAPSHOT [--region R]
  %[1]s cloud aws rds create ACCOUNT ID --engine E --class C [--username U] [--storage GB] [--multi-az] [--cluster] [--confirm] [--env E] [--project P]
  %[1]s cloud aws elasticache list|clusters|show|cluster|snapshots|snapshot|reboot|delete|delete-cluster ACCOUNT [ID] [--confirm-id ID] [--final-snapshot NAME] [--region R]
  %[1]s cloud aws snapshots list|create|delete ACCOUNT [SNAPSHOT] [--volume V | --instance I] [--exclude-boot] [--description D] [--region R]
  %[1]s cloud aws metrics ACCOUNT INSTANCE [--names CPUUtilization,NetworkIn] [--hours N] [--region R]
  %[1]s cloud aws logs groups|events ACCOUNT [GROUP] [--prefix P] [--filter PATTERN] [--hours N] [--limit N] [--region R]
  %[1]s cloud aws idle ACCOUNT [--region R]
  %[1]s cloud aws audit ACCOUNT RESOURCE [--region R]
  %[1]s cloud aws guard ACCOUNT [--kill-switch on|off]
  %[1]s cloud aws cleanup-plan ACCOUNT [--region R]

Every command takes --json. Every service is off until enabled in runtime settings. "secrets put" reads the value from %[2]s.
"cleanup-plan" and "idle" are dry runs and never delete or stop anything. "snapshots delete" only removes snapshots created here.
`, prog, cloudAWSValueEnv)
}

func isCloudAWSService(v string) bool {
	switch v {
	case "ecr", "acm", "secrets", "rds", "elasticache", "snapshots", "metrics", "logs", "idle", "audit", "guard", "cleanup-plan":
		return true
	}
	return false
}

func runCloudAWSServices(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var region, env, project, path, kind, killSwitch, engine, class, username, dbName string
	var storage, hours, limit int
	var volume, instance, names, prefix, filter, description string
	var excludeBoot bool
	var confirmID, finalSnapshot string
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
		fs.StringVar(&volume, "volume", "", "volume ID, for snapshots")
		fs.StringVar(&instance, "instance", "", "instance ID, for snapshots create")
		fs.BoolVar(&excludeBoot, "exclude-boot", false, "skip the boot volume, for snapshots create")
		fs.StringVar(&description, "description", "", "snapshot description")
		fs.StringVar(&names, "names", "", "comma separated metric names, for metrics")
		fs.IntVar(&hours, "hours", 0, "window in hours, for metrics and logs events")
		fs.IntVar(&limit, "limit", 0, "most rows, for logs")
		fs.StringVar(&prefix, "prefix", "", "log group name prefix")
		fs.StringVar(&filter, "filter", "", "log filter pattern")
		fs.StringVar(&confirmID, "confirm-id", "", "repeat the resource id to confirm an elasticache delete")
		fs.StringVar(&finalSnapshot, "final-snapshot", "", "final snapshot name for an elasticache delete")
		fs.Var(&alts, "alt", "alternative certificate name, repeatable")
	}
	label := "cloud aws " + verb
	return cloudCall(prog, label, args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
		sub := ""
		if verb == "ecr" || verb == "acm" || verb == "secrets" || verb == "rds" || verb == "elasticache" || verb == "snapshots" || verb == "logs" {
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
		if verb == "elasticache" {
			switch sub {
			case "list", "clusters", "show", "cluster", "snapshots", "snapshot", "reboot", "delete", "delete-cluster":
			default:
				return nil, nil, fmt.Errorf("unknown subcommand %q", sub)
			}
		}
		if verb == "rds" {
			switch sub {
			case "list", "show", "snapshot", "stop", "start", "adopt", "connect", "create",
				"clusters", "cluster-snapshots", "cluster-snapshot", "cluster-stop", "cluster-start", "cluster-snapshot-delete":
			default:
				return nil, nil, fmt.Errorf("unknown subcommand %q", sub)
			}
		}
		switch verb + " " + sub {
		case "snapshots list", "snapshots create", "snapshots delete", "logs groups", "logs events", "metrics ", "idle ":
		default:
			if verb == "snapshots" || verb == "logs" {
				return nil, nil, fmt.Errorf("unknown subcommand %q", sub)
			}
		}
		var err error
		switch verb + " " + sub {
		case "snapshots list":
			if volume != "" {
				q.Set("volume", volume)
			}
			err = call("GET", withQ(base+"ebs/snapshots"), nil)
			return out, table([]string{"ID", "VOLUME", "STATE", "GB", "MANAGED", "EXPIRES"}, "snapshots", "id", "volume_id", "state", "size_gb", "managed", "expires_at"), err
		case "snapshots create":
			if (volume == "") == (instance == "") {
				return nil, nil, fmt.Errorf("exactly one of --volume or --instance is required")
			}
			err = call("POST", base+"ebs/snapshots", cloudRow{"volume_id": volume, "instance_id": instance, "exclude_boot": excludeBoot, "description": description, "region": region, "env": env, "project": project})
			return out, table([]string{"ID", "VOLUME"}, "snapshots", "id", "volume_id"), err
		case "snapshots delete":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a snapshot ID is required")
			}
			err = call("DELETE", withQ(base+"ebs/snapshots/"+url.PathEscape(rest[0])), nil)
			return out, func() { _, _ = fmt.Fprintln(stdout, "deleted", rest[0]) }, err
		case "metrics ":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("an instance ID is required")
			}
			q.Set("instance_id", rest[0])
			if names != "" {
				q.Set("names", names)
			}
			if hours > 0 {
				q.Set("hours", fmt.Sprint(hours))
			}
			err = call("GET", withQ(base+"cloudwatch/metrics"), nil)
			return out, func() {
				for _, row := range rowsOf(out["series"]) {
					pts := rowsOf(row["points"])
					last := any("-")
					if len(pts) > 0 {
						last = pts[len(pts)-1]["v"]
					}
					_, _ = fmt.Fprintf(stdout, "%v\t%d points\tlast %v\n", row["name"], len(pts), last)
				}
			}, err
		case "logs groups":
			if prefix != "" {
				q.Set("prefix", prefix)
			}
			if limit > 0 {
				q.Set("limit", fmt.Sprint(limit))
			}
			err = call("GET", withQ(base+"cloudwatch/log-groups"), nil)
			return out, table([]string{"GROUP", "RETENTION DAYS", "BYTES"}, "groups", "name", "retention_days", "stored_bytes"), err
		case "logs events":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a log group is required")
			}
			q.Set("group", rest[0])
			if filter != "" {
				q.Set("filter", filter)
			}
			if hours > 0 {
				q.Set("hours", fmt.Sprint(hours))
			}
			if limit > 0 {
				q.Set("limit", fmt.Sprint(limit))
			}
			err = call("GET", withQ(base+"cloudwatch/log-events"), nil)
			return out, table([]string{"TIME", "STREAM", "MESSAGE"}, "events", "ts", "stream", "message"), err
		case "idle ":
			err = call("GET", withQ(base+"idle"), nil)
			return out, table([]string{"KIND", "ID", "ACTION", "EST USD/MO", "DETAIL"}, "suggestions", "kind", "id", "action", "est_monthly_usd", "detail"), err
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
		case "elasticache list":
			err = call("GET", withQ(base+"elasticache/replication-groups"), nil)
			return out, table([]string{"ID", "STATUS", "NODE TYPE", "ENDPOINT", "MANAGED"}, "replication_groups", "id", "status", "node_type", "primary_endpoint", "managed"), err
		case "elasticache clusters":
			err = call("GET", withQ(base+"elasticache/clusters"), nil)
			return out, table([]string{"ID", "ENGINE", "STATUS", "NODE TYPE", "GROUP", "MANAGED"}, "clusters", "id", "engine", "status", "node_type", "replication_group_id", "managed"), err
		case "elasticache snapshots":
			err = call("GET", withQ(base+"elasticache/snapshots"), nil)
			return out, table([]string{"NAME", "GROUP", "ENGINE", "STATUS", "SOURCE"}, "snapshots", "name", "replication_group_id", "engine", "status", "source"), err
		case "elasticache show", "elasticache cluster", "elasticache snapshot", "elasticache reboot", "elasticache delete", "elasticache delete-cluster":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("an ID is required")
			}
			id := url.PathEscape(rest[0])
			switch sub {
			case "show":
				err = call("GET", withQ(base+"elasticache/replication-groups/"+id), nil)
			case "cluster":
				err = call("GET", withQ(base+"elasticache/clusters/"+id), nil)
			case "snapshot":
				err = call("POST", base+"elasticache/snapshots", cloudRow{"replication_group": rest[0], "region": region, "env": env, "project": project})
			case "reboot":
				err = call("POST", withQ(base+"elasticache/clusters/"+id+"/reboot"), cloudRow{})
			default:
				q.Set("confirm", confirmID)
				if finalSnapshot != "" {
					q.Set("final_snapshot", finalSnapshot)
				}
				kind := "replication-groups/"
				if sub == "delete-cluster" {
					kind = "clusters/"
				}
				err = call("DELETE", withQ(base+"elasticache/"+kind+id), nil)
			}
			return out, func() {
				printCloudTable(stdout, []string{"ID", "STATUS", "ACTION"}, []cloudRow{out}, "id", "status", "action")
			}, err
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
		case "rds clusters":
			err = call("GET", withQ(base+"rds/clusters"), nil)
			return out, table([]string{"ID", "ENGINE", "STATUS", "MANAGED", "AUTO RESTART"}, "clusters", "id", "engine", "status", "managed", "auto_restart_at"), err
		case "rds cluster-snapshots":
			err = call("GET", withQ(base+"rds/cluster-snapshots"), nil)
			return out, table([]string{"ID", "CLUSTER", "STATUS", "TYPE", "MANAGED"}, "snapshots", "id", "cluster_id", "status", "type", "managed"), err
		case "rds cluster-snapshot", "rds cluster-stop", "rds cluster-start":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a cluster ID is required")
			}
			err = call("POST", base+"rds/clusters/"+url.PathEscape(rest[0])+"/actions", cloudRow{"action": strings.TrimPrefix(sub, "cluster-"), "region": region, "env": env, "project": project})
			return out, func() {
				printCloudTable(stdout, []string{"ID", "ACTION", "SNAPSHOT", "AUTO RESTART DAYS"}, []cloudRow{out}, "id", "action", "snapshot_id", "auto_restart_days")
			}, err
		case "rds cluster-snapshot-delete":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a snapshot ID is required")
			}
			err = call("DELETE", withQ(base+"rds/cluster-snapshots/"+url.PathEscape(rest[0])), nil)
			return out, func() { _, _ = fmt.Fprintln(stdout, "deleted") }, err
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
				printCloudTable(stdout, []string{"SNAPSHOT EXPIRED", "VOLUME"}, rowsOf(out["snapshot_expired"]), "id", "volume_id")
				printCloudTable(stdout, []string{"IDLE", "ID", "ACTION", "EST USD/MO"}, rowsOf(out["idle"]), "kind", "id", "action", "est_monthly_usd")
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
