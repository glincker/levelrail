#!/usr/bin/env python3
"""Links the first mention of curated terms to their canonical docs page."""
import glob, os, re

TERMS = {
    r"join tokens?": "multi-node", r"multi-node": "multi-node",
    r"load balanc(?:er|ers|ing)": "load-balancing", r"canary deploys?": "canary-deploys",
    r"scheduled deploys?": "scheduled-deploys", r"scheduled jobs?": "scheduled-jobs",
    r"preview environments?": "previews", r"sleep when idle": "sleep-when-idle",
    r"image auto-update": "image-auto-update", r"deploy safety": "deploy-safety",
    r"deploy failures": "deploy-failures", r"compose apps?": "compose-apps",
    r"managed databases?": "managing-databases", r"external databases?": "external-databases",
    r"database upgrades?": "database-upgrades", r"control plane backups?": "control-plane-backup",
    r"disaster recovery": "disaster-recovery", r"object storage": "object-storage",
    r"network shares?": "network-shares", r"log archive": "log-archive",
    r"log explorer": "log-explorer", r"data export": "data-export",
    r"master key rotation": "master-key-rotation", r"emergency access": "emergency-access",
    r"security center": "security-center", r"exposure audit": "exposure-audit",
    r"host firewall": "host-firewall", r"threat model": "threat-model",
    r"feature flags?": "feature-flags", r"command palette": "command-palette",
    r"cost estimate": "cost-estimate", r"resource advisor": "resource-advisor",
    r"status page": "status-page", r"attention center": "attention-center",
    r"build nodes?": "build-node-routing", r"node provisioning": "node-provisioning",
    r"network topology": "network-topology", r"platform as code": "platform-as-code",
    r"git integrations?": "git-integrations", r"OIDC federation": "pipelines-oidc",
    r"MCP recipes": "mcp-recipes", r"write mode": "mcp", r"AI control": "ai-control",
    r"teams and roles": "teams-and-roles",
    r"standalone domains?": "standalone-domains", r"domain traffic controls": "domain-traffic-controls",
    r"supply chain visibility": "supply-chain", r"deploy previews?": "deploy-previews",
    r"deploy status badge": "deploy-status-badge", r"chat deploy approvals": "chat-deploy-approvals",
    r"CLI reference": "cli-reference", r"API guard": "docker-access",
    r"cloud integrations": "cloud-integrations", r"service templates?": "templates-and-registry",
    r"template catalog": "templates-and-registry", r"starter kits?": "templates",
    r"app spec": "app-spec-reference",
}
MAX_PER_PAGE = 8
pages = {os.path.relpath(f, "docs")[:-3] for f in glob.glob("docs/**/*.md", recursive=True)}
compiled = [(re.compile(r"(?<![\w/`-])(" + k + r")(?![\w`-])", re.I), v) for k, v in TERMS.items()]
total = 0
for f in sorted(glob.glob("docs/**/*.md", recursive=True)):
    slug = os.path.relpath(f, "docs")[:-3]
    if slug.startswith(("self-host/", "announcements/")) or slug in ("index", "all-pages"):
        continue
    text = open(f).read()
    fm = re.match(r"---\n[\s\S]*?\n---\n", text)
    head, body = (text[: fm.end()], text[fm.end():]) if fm else ("", text)
    already = set(re.findall(r"\]\(/([\w/-]+)(?:#[\w-]+)?\)", body))
    parts = re.split(r"(```[\s\S]*?```)", body)
    done, n = set(), 0
    for i in range(0, len(parts), 2):
        out = []
        for line in parts[i].split("\n"):
            s = line.lstrip()
            if s.startswith(("#", "<", "|", ">", "!")) or "href=" in line:
                out.append(line); continue
            segs = re.split(r"(`[^`]*`|\[[^\]]*\]\([^)]*\)|\*\*[^*]*\*\*)", line)
            for j in range(0, len(segs), 2):
                for rx, target in compiled:
                    if n >= MAX_PER_PAGE or target == slug or target in already or target in done or target not in pages:
                        continue
                    m = rx.search(segs[j])
                    if m:
                        segs[j] = segs[j][: m.start()] + f"[{m.group(1)}](/{target})" + segs[j][m.end():]
                        done.add(target); n += 1
            out.append("".join(segs))
        parts[i] = "\n".join(out)
    if n:
        open(f, "w").write(head + "".join(parts)); total += n
print("links added:", total)
