import { stripAnsiCodes } from './ansi'

export const REDACTED = '[REDACTED]'

// Env var values shorter than this are too likely to be common words
// ("true", "3000") and would shred the report if replaced everywhere.
const MIN_LITERAL_LEN = Number(import.meta.env.VITE_REPORT_MIN_SECRET_LEN ?? 6)

const SENSITIVE_KEY =
  '[\\w.-]*(?:key|secret|token|passw(?:or)?d|pwd|credential|auth)[\\w.-]*'

const PATTERNS: [RegExp, string][] = [
  // Before the key/value rule, which would otherwise eat only the word Bearer.
  [/(Bearer\s+)[A-Za-z0-9._~+/=-]{8,}/gi, `$1${REDACTED}`],
  // KEY=value, KEY: value and "key": "value" where the key looks sensitive.
  [
    new RegExp(
      `(["']?${SENSITIVE_KEY}["']?\\s*[=:]\\s*)("[^"]*"|'[^']*'|[^\\s,;&]+)`,
      'gi',
    ),
    `$1${REDACTED}`,
  ],
  // user:pass@host in URLs.
  [/(\w+:\/\/[^\s:@/]+:)[^\s@/]+(@)/g, `$1${REDACTED}$2`],
  // Well-known token shapes that appear without a key name.
  [
    /\b(?:gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{16,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{10,}|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{5,})\b/g,
    REDACTED,
  ],
]

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// redactSecrets masks sensitive key/value pairs, token shapes, URL
// credentials and any literal value from the app's env list.
export function redactSecrets(
  text: string,
  secretValues: readonly string[] = [],
): string {
  let out = text
  for (const value of secretValues) {
    if (value.length >= MIN_LITERAL_LEN) {
      out = out.replace(new RegExp(escapeRegExp(value), 'g'), REDACTED)
    }
  }
  for (const [pattern, replacement] of PATTERNS) {
    out = out.replace(pattern, replacement)
  }
  return out
}

export interface DeployReportInput {
  appName: string
  deployId: string
  stageLabel?: string
  error?: string
  causeTitle?: string
  causeHint?: string
  lines: readonly string[]
  config: Record<string, string | undefined>
  /** Env var values to mask wherever they appear. */
  secretValues?: readonly string[]
}

const LOG_TAIL = Number(import.meta.env.VITE_REPORT_LOG_LINES ?? 100)

// buildDeployReport renders a Markdown bug report; everything user-sourced
// passes through redactSecrets so the copy is safe to paste in public.
export function buildDeployReport(input: DeployReportInput): string {
  const secrets = input.secretValues ?? []
  const clean = (s: string) => redactSecrets(s, secrets)
  const tail = input.lines.slice(-LOG_TAIL).map((l) => clean(stripAnsiCodes(l)))
  const out: string[] = [
    `# Deploy report: ${input.appName}`,
    '',
    `- App: ${input.appName}`,
    `- Deploy id: ${input.deployId}`,
  ]
  if (input.stageLabel) out.push(`- Failing stage: ${input.stageLabel}`)
  if (input.error) {
    out.push('', '## Error', '', '```', clean(input.error), '```')
  }
  if (input.causeTitle) {
    out.push('', '## Likely cause', '', `${input.causeTitle}`)
    if (input.causeHint) out.push('', input.causeHint)
  }
  const configRows = Object.entries(input.config).filter(
    (e): e is [string, string] => Boolean(e[1]),
  )
  if (configRows.length > 0) {
    out.push('', '## Config summary', '')
    for (const [k, v] of configRows) out.push(`- ${k}: ${clean(v)}`)
  }
  out.push(
    '',
    `## Last ${tail.length} log lines`,
    '',
    '```',
    ...tail,
    '```',
    '',
  )
  return out.join('\n')
}
