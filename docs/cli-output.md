---
title: "CLI output formats"
description: Output formats for every list and get command, and how jsonpath maps to JMESPath.
"og:url": "https://docs.levelrail.com/cli-output"
---

# CLI output formats

Every command that returns data takes `--output` (or `-o`) and `--query`.

| Value | What you get |
| --- | --- |
| `table` | The command's own table (default). |
| `wide` | Same as `table`; with `--query` it prints every column. |
| `json` | Indented JSON. `--json` is shorthand. |
| `yaml` | The same value as YAML. |
| `text` | Tab separated values, no header, for awk or cut. |
| `name` | One name (or id, or slug) per line. |
| `custom-columns=NAME:.name,STATUS:.status` | A table with the headers and paths you choose. A missing value prints `<none>`. |
| `jsonpath={.items[*].name}` | A kubectl style template. |

`--query` takes a JMESPath expression and runs first; the format then renders the projected value.

## jsonpath and JMESPath

The jsonpath support is a small subset: `.key`, `[n]`, `[*]`, quoted literals such as `{"\n"}`, and `{range PATH}...{end}`. Filters and recursive descent are not supported; use `--query` for those.

| Goal | jsonpath | JMESPath (`--query`) |
| --- | --- | --- |
| All names | `{.[*].name}` | `[].name` |
| First item | `{.[0]}` | `[0]` |
| Nested field | `{.status.phase}` | `status.phase` |
| One line per item | `{range .[*]}{.name}{"\n"}{end}` | `[].name` with `-o text` |
| Filter | not supported | `[?status=='running'].name` |

Multiple jsonpath matches are joined by a space. Objects and arrays print as compact JSON.
