<script setup lang="ts">
import { ref } from 'vue'
import TerminalDemo from './TerminalDemo.vue'

type LineKind = 'command' | 'output' | 'success'
interface TermLine {
  kind: LineKind
  text: string
}

interface FeatureTab {
  id: string
  label: string
  summary: string
  filename: string
  codeLines: string[]
  terminalTitle: string
  terminalAria: string
  lines: TermLine[]
}

// Every tab is grounded in a specific CLAUDE.md section (noted per tab
// below); none of this invents command syntax that section doesn't cover.
const tabs: FeatureTab[] = [
  {
    id: 'deploy',
    label: 'Deploy',
    summary: 'Push to git. Levelrail builds from app.yaml and only cuts traffic once the readiness probe passes.',
    filename: 'app.yaml',
    // Section 4.9's app spec example, trimmed to the build/domain/health/strategy fields.
    codeLines: [
      'version: 1',
      'services:',
      '  web:',
      '    build:',
      '      type: dockerfile',
      '      path: ./Dockerfile',
      '    domains:',
      '      - app.example.com',
      '    port: 3000',
      '    health:',
      '      readiness: { path: /healthz, interval: 5s, timeout: 2s }',
      '    strategy: rolling',
    ],
    terminalTitle: 'git push → deploy',
    terminalAria:
      'Terminal recording: pushing to git, building through BuildKit, and cutting traffic over once the readiness probe passes',
    lines: [
      { kind: 'command', text: 'git push origin main' },
      { kind: 'output', text: 'webhook received: commit a3f91c2 on main' },
      { kind: 'output', text: 'building web (Dockerfile) via BuildKit' },
      { kind: 'output', text: 'GET /healthz -> 200 OK (2/2 consecutive)' },
      { kind: 'success', text: 'web is live at https://app.example.com' },
    ],
  },
  {
    id: 'rollback',
    label: 'Rollback',
    summary: 'The previous N images stay pinned, so garbage collection can never orphan a rollback target.',
    filename: 'rollback.sh',
    // Phase 1 rollback guarantee + the documented `levelrail rollback` CLI command.
    codeLines: ["# previous images stay pinned,", "# so GC can't orphan a rollback target", '$ levelrail rollback web'],
    terminalTitle: 'rollback',
    terminalAria: 'Terminal recording: rolling back an app to its previously pinned image',
    lines: [
      { kind: 'command', text: 'levelrail rollback web' },
      { kind: 'output', text: 'image web:f81c0ad is still pinned (gc-safe)' },
      { kind: 'output', text: 'rolling back web to f81c0ad' },
      { kind: 'success', text: 'web is live on f81c0ad again' },
    ],
  },
  {
    id: 'observability',
    label: 'Observability',
    summary: 'Node-local metrics at 15s resolution and full-text log search, no separate Grafana or Loki install.',
    filename: 'prometheus.yml',
    // Section 4.8: node-local metrics/logs, Prometheus remote read endpoint.
    codeLines: ['# point any Prometheus at the', '# built-in remote read endpoint', 'remote_read:', '  - url: https://levelrail.local/api/v1/metrics/read'],
    terminalTitle: 'metrics + logs',
    terminalAria: 'Terminal recording: querying node-local metrics and logs without a separate observability stack',
    lines: [
      { kind: 'output', text: 'node-local metrics store: 15s resolution, 15d retention' },
      { kind: 'output', text: 'searching logs: full text, across every node' },
      { kind: 'success', text: 'deploy marker overlaid on the CPU chart at 03:14' },
    ],
  },
  {
    id: 'databases',
    label: 'Databases',
    summary: 'Managed Postgres, Redis, and more, declared alongside the app that uses them.',
    filename: 'app.yaml',
    // Section 4.9's databases: block, verbatim.
    codeLines: ['databases:', '  main:', '    engine: postgres', '    version: "16"', '    backup: { schedule: "0 3 * * *", retain: 7 }'],
    terminalTitle: 'managed database',
    terminalAria: 'Terminal recording: provisioning a managed Postgres database and injecting its connection string',
    lines: [
      { kind: 'output', text: 'provisioning managed postgres (main)' },
      { kind: 'output', text: 'volume attached, backup scheduled: 0 3 * * * retain 7' },
      { kind: 'success', text: 'DATABASE_URL injected into web as an encrypted secret' },
    ],
  },
  {
    id: 'multi-node',
    label: 'Multi-node',
    summary: 'A WireGuard mesh gives every node a peer, with stable internal DNS for apps across machines.',
    filename: 'mesh.conf',
    // Section 4.6: WireGuard mesh, peer config distributed by the control plane.
    codeLines: ['# every node gets a wireguard peer;', '# the control plane distributes config', '[Peer]', 'AllowedIPs = 10.42.0.3/32'],
    terminalTitle: 'multi-node',
    terminalAria: 'Terminal recording: enrolling a second node and moving an app to it over the WireGuard mesh',
    lines: [
      { kind: 'output', text: 'agent enrolls via one-time join token, issues client cert' },
      { kind: 'output', text: 'wireguard peer added: node-02 (10.42.0.3)' },
      { kind: 'success', text: 'web.internal resolves across both nodes' },
    ],
  },
  {
    id: 'ai-api',
    label: 'AI-ready API',
    summary: 'MCP tools are a thin wrapper over the same HTTP API the dashboard runs on.',
    filename: 'mcp-tools',
    // Section 4.11: MCP server over the platform API, tool list includes diagnose/explain.
    codeLines: ['# same HTTP API the dashboard runs on,', '# exposed as MCP tools for an AI client', 'mcp.call("diagnose_crashloop", app="web")'],
    terminalTitle: 'ai tools',
    terminalAria: 'Terminal recording: an AI client calling an MCP tool to diagnose a crashlooping app',
    lines: [
      { kind: 'output', text: '144 MCP tools (beta), same contract as the HTTP API' },
      { kind: 'output', text: 'diagnosing crashloop: last 200 lines surfaced automatically' },
      { kind: 'success', text: 'explain_failed_build: readiness probe timed out after 3 attempts' },
    ],
  },
]

const activeTab = ref(0)

interface CodeToken {
  text: string
  cls: 'tok-plain' | 'tok-key' | 'tok-punct' | 'tok-string' | 'tok-comment' | 'tok-prompt'
}

function tokenizeStrings(raw: string): CodeToken[] {
  if (!raw) return []
  const parts = raw.split(/("(?:[^"\\]|\\.)*")/)
  return parts
    .filter((part) => part.length > 0)
    .map((part) => ({ text: part, cls: part.startsWith('"') ? 'tok-string' : 'tok-plain' }))
}

// Small hand-rolled tokenizer (not a real highlighter): comments, YAML
// keys, list dashes, and a shell prompt get a span class each, everything
// else falls through to plain text. Good enough for a handful of lines.
function tokenizeLine(raw: string): CodeToken[] {
  const trimmed = raw.trimStart()
  const indent = raw.slice(0, raw.length - trimmed.length)
  const tokens: CodeToken[] = indent ? [{ text: indent, cls: 'tok-plain' }] : []

  if (trimmed.startsWith('#')) {
    tokens.push({ text: trimmed, cls: 'tok-comment' })
    return tokens
  }
  if (trimmed.startsWith('$ ')) {
    tokens.push({ text: '$ ', cls: 'tok-prompt' })
    tokens.push(...tokenizeStrings(trimmed.slice(2)))
    return tokens
  }
  const listMatch = trimmed.match(/^(- )(.*)$/)
  if (listMatch) {
    tokens.push({ text: listMatch[1], cls: 'tok-punct' })
    tokens.push(...tokenizeStrings(listMatch[2]))
    return tokens
  }
  const keyMatch = trimmed.match(/^([\w./-]+)(:)(\s?)(.*)$/)
  if (keyMatch) {
    const [, key, colon, space, rest] = keyMatch
    tokens.push({ text: key, cls: 'tok-key' })
    tokens.push({ text: colon, cls: 'tok-punct' })
    if (space) tokens.push({ text: space, cls: 'tok-plain' })
    if (rest) tokens.push(...tokenizeStrings(rest))
    return tokens
  }
  tokens.push(...tokenizeStrings(trimmed))
  return tokens
}

function selectTab(index: number) {
  activeTab.value = index
}

function onTabKeydown(event: KeyboardEvent, index: number) {
  if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return
  event.preventDefault()
  const delta = event.key === 'ArrowRight' ? 1 : -1
  const next = (index + delta + tabs.length) % tabs.length
  activeTab.value = next
  const el = document.getElementById(`feature-tab-${tabs[next].id}`)
  el?.focus()
}
</script>

<template>
  <div class="feature-tabs">
    <div class="feature-tabs__pills" role="tablist" aria-label="Levelrail capabilities">
      <button
        v-for="(tab, i) in tabs"
        :id="`feature-tab-${tab.id}`"
        :key="tab.id"
        type="button"
        role="tab"
        class="feature-tabs__pill"
        :class="{ 'feature-tabs__pill--active': activeTab === i }"
        :aria-selected="activeTab === i"
        :aria-controls="`feature-panel-${tab.id}`"
        :tabindex="activeTab === i ? 0 : -1"
        @click="selectTab(i)"
        @keydown="onTabKeydown($event, i)"
      >
        {{ tab.label }}
      </button>
    </div>

    <p class="feature-tabs__summary">{{ tabs[activeTab].summary }}</p>

    <div
      :id="`feature-panel-${tabs[activeTab].id}`"
      class="feature-tabs__panels"
      role="tabpanel"
      :aria-labelledby="`feature-tab-${tabs[activeTab].id}`"
    >
      <div class="code-editor">
        <div class="code-editor__bar" aria-hidden="true">
          <span class="code-editor__filename">{{ tabs[activeTab].filename }}</span>
        </div>
        <pre class="code-editor__body"><code
          ><div
            v-for="(raw, li) in tabs[activeTab].codeLines"
            :key="li"
            class="code-editor__line"
          ><span
            v-for="(tok, ti) in tokenizeLine(raw)"
            :key="ti"
            :class="tok.cls"
            >{{ tok.text }}</span
          ></div></code
        ></pre>
      </div>

      <TerminalDemo
        :key="tabs[activeTab].id"
        :lines="tabs[activeTab].lines"
        :title="tabs[activeTab].terminalTitle"
        :aria-label="tabs[activeTab].terminalAria"
      />
    </div>
  </div>
</template>
