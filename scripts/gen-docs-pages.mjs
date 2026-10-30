// Generates the Mintlify pages that have no hand-written source: the
// self-host template gallery and the changelog. Run from the repo root.
import fs from 'node:fs'

const kindLabel = { generated: 'Generated at deploy', required: 'You must provide', preset: 'Preset in the template' }
const templates = JSON.parse(fs.readFileSync('scripts/gen-template-gallery/templates.json', 'utf8'))
const esc = (s) => String(s).replace(/\|/g, '\\|').replace(/[<>{}]/g, (c) => `&#${c.charCodeAt(0)};`)
const q = (s) => JSON.stringify(String(s))

const shotDir = 'docs/assets/screenshots'
const shot = (file, title) =>
  fs.existsSync(`${shotDir}/${file}-light.png`)
    ? `  <img src="/assets/screenshots/${file}-light.png" alt=${q(title)} className="block dark:hidden" />\n  <img src="/assets/screenshots/${file}.png" alt=${q(title)} className="hidden dark:block" />`
    : `  <img src="/assets/screenshots/${file}.png" alt=${q(title)} />`

fs.mkdirSync('docs/self-host', { recursive: true })
for (const f of fs.readdirSync('docs/self-host')) if (f.endsWith('.mdx')) fs.unlinkSync(`docs/self-host/${f}`)

const byCat = new Map()
for (const t of templates) {
  if (!byCat.has(t.category)) byCat.set(t.category, [])
  byCat.get(t.category).push(t)
}

for (const t of templates) {
  const related = (byCat.get(t.category) ?? []).filter((r) => r.id !== t.id).slice(0, 5)
  const svc = t.services
    .map(
      (s) =>
        `| \`${s.name}\` | \`${esc(s.image)}\` | ${s.ports.length ? s.ports.join(', ') : 'none'} | ${
          s.volumes.length ? s.volumes.map((v) => `\`${esc(v)}\``).join('<br />') : 'none'
        } |`,
    )
    .join('\n')
  const env = t.env.length
    ? `| Service | Variable | Value |\n| --- | --- | --- |\n${t.env
        .map((e) => `| \`${e.service}\` | \`${e.name}\` | ${kindLabel[e.kind] ?? e.kind} |`)
        .join('\n')}`
    : 'This template sets no environment variables.'
  const req = t.requirements
  const reqLevel = { standard: 'Standard', review: 'Review before deploying', elevated: 'Elevated access' }[req.level] ?? req.level
  const reqFindings = req.findings.length
    ? req.findings
        .map((f) => `- ${esc(f.message)}${f.services?.length ? ` (${f.services.map((n) => `\`${n}\``).join(', ')})` : ''}${f.detail ? `: ${esc(f.detail)}` : ''}.`)
        .join('\n')
    : '- Nothing beyond a standard container: no extra privileges, host mounts or published ports.'
  const reqSection = `## Security and requirements

Level: **${reqLevel}**.

What this template needs:

${reqFindings}

What Levelrail adds around it:

${req.protections.map((p) => `- ${esc(p.label)}.`).join('\n')}
`
  const cmd = t.needsConfig
    ? `levelrail-cli templates get ${t.id} --output json | jq -r .compose > ${t.id}.compose.yaml\n# edit ${t.id}.compose.yaml to set the variables you must provide\nlevelrail-cli apps deploy-compose my-${t.id} --file ${t.id}.compose.yaml`
    : `levelrail-cli templates deploy ${t.id} --name my-${t.id}`
  const out = `---
title: ${q(`Self-host ${t.name} with Docker`)}
sidebarTitle: ${q(t.name)}
description: ${q(t.slogan)}
"og:url": "https://docs.levelrail.com/${'self-host/'+t.id}"
---

${esc(t.slogan)}

This page describes the Levelrail template for ${esc(t.name)} (${esc(t.category)}). It deploys the services below as one Docker Compose app on your own server.${t.docUrl ? ` Project site: ${t.docUrl}.` : ''}
${t.memoryMiB ? `\nRecommended memory: about ${t.memoryMiB} MiB.\n` : ''}${t.gpu ? '\n<Warning>This template reserves an NVIDIA GPU, so it needs a node with a GPU and the NVIDIA container runtime.</Warning>\n' : ''}
## Services, ports and volumes

| Service | Image | Container ports | Volumes |
| --- | --- | --- | --- |
${svc}

## Environment variables

${env}

Passwords and keys marked as generated are created for you when the app is deployed and stored as secrets. Values are not shown here.

${reqSection}${t.needsConfig ? '\n<Warning>This template needs configuration before it can deploy. Open it in the dashboard, set each variable marked "You must provide" in the Compose body, then deploy.</Warning>\n' : ''}
## Deploy ${esc(t.name)} with Levelrail

In the dashboard, open **Apps**, choose **New app**, then **Browse templates**, and select ${esc(t.name)}. Review the Compose body and deploy.

With the CLI:

\`\`\`bash
${cmd}
\`\`\`

See the [service template catalog](/templates-and-registry) for how templates work, and [Getting started](/getting-started) if you have not installed Levelrail yet.
${related.length ? `\n## More ${esc(t.category)} templates\n\n${related.map((r) => `- [${esc(r.name)}](/self-host/${r.id})`).join('\n')}\n` : ''}
All templates are listed in the [self-host gallery](/self-host).
`
  fs.writeFileSync(`docs/self-host/${t.id}.mdx`, out)
}

const cats = [...byCat.keys()].sort()
const index = `---
title: "Self-host gallery"
description: Browse self-hostable open-source apps you can deploy with Levelrail templates, with the services, ports, volumes and environment variables for each.
"og:url": "https://docs.levelrail.com/self-host"
---

${templates.length} open-source apps you can run on your own server with a Levelrail template. Each page lists the services, ports, volumes and environment variables the template sets up, and how to deploy it. For how templates work, see the [service template catalog](/templates-and-registry).

${cats
  .map(
    (c) =>
      `## ${c}\n\n${byCat
        .get(c)
        .sort((a, b) => a.name.localeCompare(b.name))
        .map((t) => `- [${esc(t.name)}](/self-host/${t.id}): ${esc(t.slogan)}`)
        .join('\n')}\n`,
  )
  .join('\n')}`
fs.writeFileSync('docs/self-host/index.mdx', index)

// Changelog from release-please's CHANGELOG.md.
const cl = fs.readFileSync('CHANGELOG.md', 'utf8')
const updates = cl
  .split(/^## /m)
  .slice(1)
  .map((e) => {
    const [head, ...rest] = e.split('\n')
    const m = head.match(/\[?([^\]\s]+)\]?(?:\([^)]*\))?\s*\((\d{4}-\d{2}-\d{2})\)/)
    if (!m) return ''
    const body = rest
      .join('\n')
      .replace(/^### (.*)$/gm, '**$1**')
      .replace(/\s*\(\[[0-9a-f]{7}\]\([^)]*\)\)/g, '')
      .replace(/\[\\?#(\d+)\]\([^)]*\)/g, '#$1')
      .replace(/[<>{}]/g, (c) => `&#${c.charCodeAt(0)};`)
      .trim()
    return `<Update label=${q(m[1])} description=${q(m[2])}>\n${body}\n</Update>`
  })
  .filter(Boolean)
fs.writeFileSync(
  'docs/changelog.mdx',
  `---\ntitle: "Changelog"\ndescription: Every Levelrail release with highlights, fixes and upgrade notes.\n"og:url": "https://docs.levelrail.com/changelog"\n---\n\n${updates.join('\n\n')}\n`,
)

// "All pages" index: every nav page with its description, so no page is an orphan
// and crawlers get one hub that links the whole docs set.
{
  const cfgNav = JSON.parse(fs.readFileSync('docs/docs.json', 'utf8'))
  const flat = (items) => items.flatMap((i) => (typeof i === 'string' ? [i] : flat(i.pages)))
  const meta = (slug) => {
    for (const ext of ['md', 'mdx']) {
      const f = `docs/${slug}.${ext}`
      if (!fs.existsSync(f)) continue
      const fm = fs.readFileSync(f, 'utf8').match(/^---\n([\s\S]*?)\n---/)?.[1] ?? ''
      const get = (k) => fm.match(new RegExp(`^${k}:\\s*"?(.*?)"?\\s*$`, 'm'))?.[1]
      return { title: get('title') ?? slug, description: get('description') ?? '' }
    }
    return null
  }
  const sections = []
  for (const tab of cfgNav.navigation.tabs) {
    if (tab.tab === 'Self-host gallery') continue
    sections.push(`## ${tab.tab}\n`)
    for (const g of tab.groups) {
      const rows = flat(g.pages)
        .filter((s) => s !== 'all-pages')
        .map((s) => [s, meta(s)])
        .filter(([, m]) => m)
        .map(([s, m]) => `- [${esc(m.title)}](/${s})${m.description ? `: ${esc(m.description)}` : ''}`)
      if (rows.length) sections.push(`### ${g.group}\n\n${rows.join('\n')}\n`)
    }
  }
  fs.writeFileSync(
    'docs/all-pages.mdx',
    `---\ntitle: "All pages"\ndescription: Every Levelrail documentation page with a one-line summary, grouped by section.\n"og:url": "https://docs.levelrail.com/all-pages"\n---\n\nThe complete index of the Levelrail docs. For a self-hosted app gallery, see the [template gallery](/self-host).\n\n${sections.join('\n')}`,
  )
}

// Product tour: every dashboard screenshot with a title, grouped by area.
{
  const shots = [
    ['Overview', [
      ['dashboard-home', 'Dashboard home', 'Health, needs-attention items and recent activity in one place.', '/attention-center'],
      ['setup-wizard', 'Setup wizard', 'The guided first-run checklist for domain, git and backups.', '/getting-started'],
      ['cli-access', 'CLI access', 'Mint a token and copy the install command for the CLI.', '/cli-reference'],
    ]],
    ['Apps and deploys', [
      ['apps-list', 'Apps', 'Every app with its status, replicas and last deploy.', '/deploying-apps'],
      ['app-overview-page', 'App overview', 'One app: status, domains, resources and quick actions.', '/deploying-apps'],
      ['app-source', 'App source', 'Connect a repository or image and pick how it builds.', '/git-integrations'],
      ['app-environment', 'Environment and secrets', 'Variables and encrypted secrets per app.', '/environment-and-secrets'],
      ['app-deploy-settings', 'Deploy settings', 'Strategy, health checks, schedules and approvals.', '/deploy-safety'],
      ['app-resources', 'Resources', 'CPU and memory limits with live usage.', '/resource-advisor'],
      ['deploy-history', 'Deploy history', 'Every deploy with its status, commit and one-click rollback.', '/deployments-page'],
      ['deployments-all', 'All deployments', 'Deploys across every app in one list.', '/deployments-page'],
      ['pipelines', 'Pipelines', 'Build, test and deploy steps that run on your own nodes.', '/pipelines'],
    ]],
    ['Observe', [
      ['app-overview', 'Metrics', 'CPU, memory, requests and deploy markers on the same chart.', '/observability'],
      ['logs', 'Logs', 'Full-text search and a live tail over the node-local log store.', '/observability'],
    ]],
    ['Data', [
      ['databases-list', 'Databases', 'Managed Postgres, MySQL, Redis and more.', '/managing-databases'],
      ['database-overview', 'Database overview', 'Connection details, backups and version for one database.', '/managing-databases'],
      ['backups', 'Backups', 'Backup targets, schedules and restore points.', '/backups-and-storage'],
      ['templates-catalog', 'Template catalog', 'One-click services from the curated catalog.', '/templates-and-registry'],
    ]],
    ['Network and nodes', [
      ['domains-list', 'Domains', 'Domains, certificates and the platform ingress settings.', '/domains-and-ingress'],
      ['load-balancer', 'Load balancer', 'Upstreams and drain behaviour across replicas.', '/load-balancing'],
      ['network-topology', 'Network topology', 'How nodes, apps and databases connect.', '/network-topology'],
      ['nodes', 'Nodes', 'Enrolled servers with health, workload kinds and drain controls.', '/multi-node'],
      ['node-providers', 'Cloud node providers', 'Connect a cloud account to create servers from the dashboard.', '/node-provisioning'],
      ['status-page-settings', 'Public status page', 'Choose which components to publish and how.', '/status-page'],
    ]],
  ]
  const body = shots
    .map(([area, items]) =>
      `## ${area}\n\n${items
        .map(
          ([file, title, desc, link]) =>
            `### ${title}\n\n${desc} Read more in [the guide](${link}).\n\n<Frame caption=${q(title)}>\n${shot(file, title)}\n</Frame>\n`,
        )
        .join('\n')}`,
    )
    .join('\n')
  fs.writeFileSync(
    'docs/product-tour.mdx',
    `---\ntitle: "Product tour"\nsidebarTitle: "Product tour"\ndescription: "A look at the Levelrail dashboard: apps, deploys, logs, databases, domains and nodes, each with a short explanation."\n"og:url": "https://docs.levelrail.com/product-tour"\n---\n\nThe screenshots below come from a real running control plane, not mockups. Each one links to the page that explains it.\n\n${body}`,
  )
}

// Keep docs.json's gallery tab in sync.
const cfg = JSON.parse(fs.readFileSync('docs/docs.json', 'utf8'))
const tabs = cfg.navigation.tabs.filter((t) => t.tab !== 'Self-host gallery')
tabs.splice(tabs.length - 1, 0, {
  tab: 'Self-host gallery',
  icon: '/assets/icons/squares-four.svg',
  groups: [
    { group: 'Gallery', pages: ['self-host/index'] },
    ...cats.map((c) => ({ group: c, pages: byCat.get(c).map((t) => `self-host/${t.id}`) })),
  ],
})
const expl = tabs.find((x) => x.tab === 'Concepts')?.groups.find((g) => g.group === 'Explanation')
if (expl && !expl.pages.includes('product-tour')) expl.pages.unshift('product-tour')
cfg.navigation.tabs = tabs
fs.writeFileSync('docs/docs.json', JSON.stringify(cfg, null, 2) + '\n')
