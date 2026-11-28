package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"

	"github.com/glincker/levelrail/internal/apiclient"
)

// runCloudAWSLightsail handles the lightsail verb. It stays off until its runtime flag is on.
func runCloudAWSLightsail(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var region, env, project, zone, blueprint, bundle, keyPair, confirmID, instance string
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&region, "region", "", "AWS region, defaults to the account region")
		fs.StringVar(&env, "env", "", "env tag for new resources")
		fs.StringVar(&project, "project", "", "project tag for new resources")
		fs.StringVar(&zone, "zone", "", "availability zone, for create")
		fs.StringVar(&blueprint, "blueprint", "", "blueprint id, for create")
		fs.StringVar(&bundle, "bundle", "", "bundle id, for create")
		fs.StringVar(&keyPair, "key-pair", "", "key pair name, for create")
		fs.StringVar(&confirmID, "confirm-id", "", "repeat the instance name to confirm a delete")
		fs.StringVar(&instance, "instance", "", "instance name, for snapshot")
	}
	return cloudCall(prog, "cloud aws lightsail", args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
		if len(pos) < 2 {
			return nil, nil, fmt.Errorf("a subcommand and an account ID are required (see help)")
		}
		sub, acct, rest := pos[0], pos[1], pos[2:]
		base := "/" + url.PathEscape(acct) + "/aws/lightsail/"
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
		var out cloudRow
		call := func(method, p string, body any) error {
			return c.CloudRequest(context.Background(), method, p, body, &out)
		}
		table := func(head []string, key string, keys ...string) func() {
			return func() { printCloudTable(stdout, head, rowsOf(out[key]), keys...) }
		}
		action := func() { printCloudTable(stdout, []string{"ID", "ACTION"}, []cloudRow{out}, "id", "action") }
		var err error
		switch sub {
		case "list":
			err = call("GET", withQ(base+"instances"), nil)
			return out, table([]string{"NAME", "STATE", "BUNDLE", "ZONE", "IP", "MANAGED"}, "instances", "name", "state", "bundle_id", "zone", "public_ip", "managed"), err
		case "bundles":
			err = call("GET", withQ(base+"bundles"), nil)
			return out, table([]string{"ID", "NAME", "USD/MO", "CPUS", "RAM GB", "DISK GB", "ALLOWED"}, "bundles", "id", "name", "price_usd", "cpus", "ram_gb", "disk_gb", "allowed"), err
		case "blueprints":
			err = call("GET", withQ(base+"blueprints"), nil)
			return out, table([]string{"ID", "NAME", "TYPE", "VERSION"}, "blueprints", "id", "name", "type", "version"), err
		case "regions":
			err = call("GET", withQ(base+"regions"), nil)
			return out, table([]string{"NAME", "DISPLAY"}, "regions", "name", "display_name"), err
		case "static-ips":
			err = call("GET", withQ(base+"static-ips"), nil)
			return out, table([]string{"NAME", "IP", "ATTACHED TO"}, "static_ips", "name", "ip", "attached_to"), err
		case "snapshots":
			err = call("GET", withQ(base+"snapshots"), nil)
			return out, table([]string{"NAME", "STATE", "INSTANCE", "GB", "MANAGED"}, "snapshots", "name", "state", "instance", "size_gb", "managed"), err
		case "snapshot":
			if instance == "" {
				return nil, nil, fmt.Errorf("--instance is required")
			}
			name := ""
			if len(rest) > 0 {
				name = rest[0]
			}
			err = call("POST", base+"snapshots", cloudRow{"instance": instance, "name": name, "region": region, "env": env, "project": project})
			return out, func() { printCloudTable(stdout, []string{"ID", "SNAPSHOT"}, []cloudRow{out}, "id", "snapshot_name") }, err
		case "snapshot-delete":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("a snapshot name is required")
			}
			err = call("DELETE", withQ(base+"snapshots/"+url.PathEscape(rest[0])), nil)
			return out, action, err
		case "create":
			if len(rest) < 1 || zone == "" || blueprint == "" || bundle == "" {
				return nil, nil, fmt.Errorf("a name, --zone, --blueprint and --bundle are required")
			}
			err = call("POST", base+"instances", cloudRow{"name": rest[0], "availability_zone": zone, "blueprint_id": blueprint, "bundle_id": bundle,
				"key_pair": keyPair, "region": region, "env": env, "project": project})
			return out, action, err
		case "show", "start", "stop", "reboot", "delete":
			if len(rest) < 1 {
				return nil, nil, fmt.Errorf("an instance name is required")
			}
			p := base + "instances/" + url.PathEscape(rest[0])
			switch sub {
			case "show":
				err = call("GET", withQ(p), nil)
				return out, func() {
					printCloudTable(stdout, []string{"NAME", "STATE", "BUNDLE", "ZONE", "IP", "MANAGED"}, []cloudRow{out}, "name", "state", "bundle_id", "zone", "public_ip", "managed")
				}, err
			case "delete":
				q.Set("confirm", confirmID)
				err = call("DELETE", withQ(p), nil)
			default:
				err = call("POST", withQ(p+"/"+sub), cloudRow{})
			}
			return out, action, err
		}
		return nil, nil, fmt.Errorf("unknown subcommand %q", sub)
	})
}
