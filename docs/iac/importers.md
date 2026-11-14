# Import from a deploy.yml or a Portainer stack

Two offline importers turn an existing deployment definition into reviewable Levelrail documents (App kinds inside a Stack). Both are dry runs: nothing is written unless you pass `--write DIR`, and existing files are kept without `--force`.

```sh
levelrail-cli import deploy-yml config/deploy.yml                 # print the documents
levelrail-cli import deploy-yml config/deploy.yml --write ./iac   # save them
levelrail-cli import portainer stack.json --stack blog --write ./iac
```

Then run `levelrail-cli stack plan NAME -f ./iac` and apply as for any stack.

## Secrets are references

Values are never read into the output. Every secret becomes a name listed in `SECRETS.md`, and the App document refers to it as a secret reference. Clear environment values that look like secrets (by name or by value) are turned into references too. Fill them at apply time with `--secret NAME=env:CI_SECRET`.

## deploy.yml

| Source field | Result |
| --- | --- |
| `service`, `image` | the main App (both required) |
| `servers.<role>.cmd` | one extra App per role with a command |
| `proxy.host`, `proxy.hosts` | App domains |
| `proxy.app_port`, `proxy.healthcheck.path` | App port and readiness path |
| `env.clear` | App env, with secret-looking values turned into references |
| `env.secret` | secret names |
| `accessories.<name>` | one App each (image, port, env, cmd) |

Fields with no equivalent (`registry`, `ssh`, `builder`, `boot`, `volumes`, server hosts and others) are listed in the notes. Any field the importer does not know is an error that names it, so nothing is dropped silently. A file with ERB tags (`<% %>`) is refused: render it first.

## Portainer

The input is a stack compose file or the JSON export of a stack (`Name`, `StackFileContent`, `Env`). Image services become Apps. Environment values and the export `Env` values are never read, only names. Services with a build context are skipped with a note, and volumes, command overrides and `env_file` entries are listed in the notes.

The size limit for the input file is `LEVELRAIL_IMPORT_FILE_MAX_BYTES`.
