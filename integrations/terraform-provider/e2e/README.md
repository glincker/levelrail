# Provider e2e scripts

One folder per resource, each with a `run.sh` and a README. They never run in CI and never download anything: you supply the instance and the provider binary.

Requirements: a throwaway local instance you can wipe, a root token for it, `tofu` or `terraform`, `curl` and `jq` on PATH, and a provider binary built with `go build -o /path/terraform-provider-levelrail .` and wired in with a CLI config whose `dev_overrides` points `registry.terraform.io/glincker/levelrail` at its folder (`TF_CLI_CONFIG_FILE`).

Safety: each script refuses to run unless `E2E_I_UNDERSTAND_THIS_USES_A_THROWAWAY_INSTANCE=1` is set and `LEVELRAIL_ENDPOINT` is a loopback address (`localhost`, `127.x.x.x` or `[::1]`). The token is read from `LEVELRAIL_TOKEN` and passed to the provider through its environment, never written to a file.

Not covered yet: apps, domains, databases, environments, environment variables, secrets, alert rules and status page resources. They follow the same `lib.sh` flow when someone adds a folder.
