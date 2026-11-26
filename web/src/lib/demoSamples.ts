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
