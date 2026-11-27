// Canned output for the read-only demo surfaces that need a real host. Values are
// generic placeholders, never read from the instance, and the UI labels them as samples.
export type DemoChatPromptId = 'crashloop' | 'slow' | 'rollback'

export const DEMO_CHAT_PROMPTS: readonly DemoChatPromptId[] = [
  'crashloop',
  'slow',
  'rollback',
]

export interface DemoTerminalLine {
  prompt: boolean
  text: string
}

export const DEMO_TERMINAL_LINES: readonly DemoTerminalLine[] = [
  { prompt: true, text: 'node --version' },
  { prompt: false, text: 'v22.11.0' },
  { prompt: true, text: 'ls' },
  { prompt: false, text: 'dist  node_modules  package.json  server.js' },
  { prompt: true, text: 'env | grep NODE_ENV' },
  { prompt: false, text: 'NODE_ENV=production' },
  { prompt: true, text: 'ps -o pid,etime,cmd' },
  { prompt: false, text: '  PID     ELAPSED CMD' },
  { prompt: false, text: '    1       02:14:09 node server.js' },
]

export const DEMO_QUERY_SQL =
  'SELECT id, email, created_at FROM users ORDER BY created_at DESC LIMIT 3;'

export const DEMO_QUERY_COLUMNS = ['id', 'email', 'created_at'] as const

export const DEMO_QUERY_ROWS: readonly (readonly string[])[] = [
  ['3', 'cora@example.com', '2026-01-14 09:12:41'],
  ['2', 'ben@example.com', '2026-01-13 17:03:08'],
  ['1', 'ada@example.com', '2026-01-12 08:55:30'],
]

// Build narration for the sample app result: generic steps, not tied to any real build.
export const DEMO_BUILD_LOG: readonly string[] = [
  'Cloning repository',
  'Detected stack, using the default build plan',
  'Installing dependencies',
  'Building image',
  'Starting container',
  'Health check passed',
  'Deploy complete',
]

// example.com is reserved for documentation, so a sample URL can never point at a real site.
export function demoSampleUrl(name: string): string {
  const slug =
    name
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9-]+/g, '-') || 'app'
  return `https://${slug}.example.com`
}

const DB_SCHEMES: Record<string, { scheme: string; port: number }> = {
  postgres: { scheme: 'postgres', port: 5432 },
  postgresql: { scheme: 'postgres', port: 5432 },
  mysql: { scheme: 'mysql', port: 3306 },
  mariadb: { scheme: 'mysql', port: 3306 },
  redis: { scheme: 'redis', port: 6379 },
  mongodb: { scheme: 'mongodb', port: 27017 },
  mongo: { scheme: 'mongodb', port: 27017 },
}

export const DEMO_MASK = '••••••••'

// The password is always masked; the sample never carries a real secret.
export function demoSampleConnection(engine: string, name: string): string {
  const e = DB_SCHEMES[engine.toLowerCase()] ?? {
    scheme: engine.toLowerCase() || 'postgres',
    port: 5432,
  }
  const db = name.trim() || 'db'
  return `${e.scheme}://app:${DEMO_MASK}@${db}:${e.port}/app`
}

export interface DemoEnvDiff {
  added: string[]
  changed: string[]
  removed: string[]
}

/** Key-level diff only, values never leave the form. */
export function diffEnvKeys(
  before: Readonly<Record<string, string>>,
  after: Readonly<Record<string, string>>,
): DemoEnvDiff {
  const added = Object.keys(after).filter((k) => !(k in before))
  const removed = Object.keys(before).filter((k) => !(k in after))
  const changed = Object.keys(after).filter(
    (k) => k in before && before[k] !== after[k],
  )
  return { added, changed, removed }
}

// Documentation address range, never a real server.
export const DEMO_SAMPLE_IP = '203.0.113.10'

export interface DemoDnsRow {
  type: 'A' | 'CNAME'
  host: string
  value: string
  verified: boolean
}

export function demoDnsRows(domain: string): DemoDnsRow[] {
  const d = domain.trim().toLowerCase() || 'app.example.com'
  return [
    { type: 'A', host: d, value: DEMO_SAMPLE_IP, verified: true },
    { type: 'CNAME', host: `www.${d}`, value: d, verified: false },
  ]
}

export const DEMO_SAMPLE_LOGS: readonly string[] = [
  'Server listening on port 3000',
  'GET /health 200 2ms',
  'GET / 200 14ms',
  'GET /api/items 200 21ms',
  'POST /api/items 201 33ms',
  'GET /favicon.ico 304 1ms',
]
