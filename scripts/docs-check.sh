#!/usr/bin/env bash
# Docs accuracy check: every `levelrail-cli ...` command in docs/*.md must parse
# against the real CLI (-h), and relative markdown links must resolve.
# Usage: scripts/docs-check.sh [--links-only|--commands-only]
# Env: LEVELRAIL_CLI=<path to a built levelrail-cli> (built into a temp dir if unset).
set -u
cd "$(dirname "$0")/.." || exit 2

mode="${1:-all}"
cli="${LEVELRAIL_CLI:-}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ "$mode" != "--links-only" ] && [ -z "$cli" ]; then
  cli="$tmp/levelrail-cli"
  go build -o "$cli" ./cmd/levelrail-cli || { echo "docs-check: cannot build the CLI" >&2; exit 2; }
fi

# Feature-gated verbs only show their flags when the feature is on.
export APP_EXPERIMENTAL="ai-chat,ai-models,load-balancer,iac,cloudflare-tunnel,access-roles,global-environments,ai-control,resource-advisor,cli-console,mobile-api,saml-scim,plugins"

python3 -I - "$mode" "$cli" <<'PY'
import os, re, subprocess, sys, glob, tempfile

mode, cli = sys.argv[1], sys.argv[2]
problems = []
docs = sorted(glob.glob("docs/*.md") + glob.glob("docs/*.mdx"))
help_cache = {}
GLOBAL_FLAGS = {"--debug", "--json", "--output", "--query", "--token", "--api-url", "--profile"}
probe_env = dict(os.environ, APP_API_URL="http://127.0.0.1:1", HOME=os.environ.get("TMPDIR", "/tmp"))

probe_cwd = tempfile.mkdtemp()
PRE_PARSED = {"--offbox", "--requests"}

def flag_defined(words, flag):
    # An undefined flag fails at parse time, before any request; -h stops a defined one early.
    p = subprocess.run([cli, *words, flag + "=x", "-h"], capture_output=True, text=True, timeout=20, env=probe_env, cwd=probe_cwd)
    return "not defined" not in (p.stdout + p.stderr)

def help_text(args):
    key = tuple(args)
    if key not in help_cache:
        p = subprocess.run([cli, *args, "-h"], capture_output=True, text=True, timeout=20, env=probe_env, cwd=probe_cwd)
        help_cache[key] = (p.stdout + p.stderr)
    return help_cache[key]

def valid(args):
    out = help_text(args)
    return "unknown command" not in out and "unknown subcommand" not in out

def check_command(path, lineno, line):
    m = re.search(r"\blevelrail-cli\b\s+(.*)", line)
    if not m or "#" in line[: m.start()]:
        return
    toks = re.split(r"\s+", re.sub(r"\$\([^)]*\)", "", m.group(1)).strip())
    words, rest = [], []
    for i, t in enumerate(toks):
        if re.fullmatch(r"[a-z0-9][a-z0-9-]*", t) and not rest:
            words.append(t)
        else:
            rest = toks[i:]
            break
    if not words:
        return
    # Longest prefix that is a real command; trailing words are positional args.
    depth = 0
    for n in range(1, len(words) + 1):
        if valid(words[:n]):
            depth = n
        else:
            break
    if depth == 0:
        problems.append(f"{path}:{lineno}: unknown command: levelrail-cli {words[0]}")
        return
    cmd = words[:depth]
    for t in rest:
        if t in ("|", ">", ">>", "&&", ";", "\\"):
            break
        fm = re.match(r"^(--[A-Za-z][A-Za-z0-9-]*)", t)
        if not fm:
            continue
        flag = fm.group(1)
        if flag in GLOBAL_FLAGS or flag in PRE_PARSED or flag == "--help":
            continue
        if not flag_defined(words, flag):
            problems.append(f"{path}:{lineno}: levelrail-cli {' '.join(words)}: flag {flag} is not defined")

def check_links(path, text):
    base = os.path.dirname(path)
    in_fence = False
    for lineno, line in enumerate(text.splitlines(), 1):
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        for m in re.finditer(r"\]\(([^)\s]+)\)", re.sub(r"`[^`]*`", "", line)):
            target = m.group(1).split("#")[0]
            if not target or re.match(r"^(https?:|mailto:|/|#|tel:)", target):
                continue
            full = os.path.normpath(os.path.join(base, target))
            cands = [full, full + ".md", full + ".mdx", os.path.join(full, "index.md")]
            if not any(os.path.exists(c) for c in cands):
                problems.append(f"{path}:{lineno}: broken link {m.group(1)}")

for path in docs:
    text = open(path, encoding="utf-8").read()
    if mode != "--commands-only":
        check_links(path, text)
    if mode == "--links-only":
        continue
    in_fence = False
    joined = []
    carry = None
    for lineno, line in enumerate(text.splitlines(), 1):
        if carry is not None:
            carry = (carry[0], carry[1] + " " + line.strip())
        else:
            carry = (lineno, line)
        if carry[1].rstrip().endswith("\\") and "```" not in line:
            carry = (carry[0], carry[1].rstrip()[:-1])
            continue
        joined.append(carry)
        carry = None
    for lineno, line in joined:
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
            continue
        if "levelrail-cli" not in line:
            continue
        if in_fence or line.lstrip().startswith("$"):
            check_command(path, lineno, line.replace("$ ", "", 1))
        else:
            for m in re.finditer(r"`(levelrail-cli [^`]+)`", line):
                check_command(path, lineno, m.group(1))

for p in problems:
    print(p)
print(f"docs-check: {len(problems)} problem(s) in {len(docs)} files")
sys.exit(1 if problems else 0)
PY
