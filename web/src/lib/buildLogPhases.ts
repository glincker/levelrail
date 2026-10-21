import type { LogLine } from '../hooks/useLogStream'
import { stripAnsiCodes } from './ansi'

export type LogPhase = 'install' | 'build' | 'push' | 'other'

export interface BuildStep {
  id: string
  label: string
  phase: LogPhase
  cached: boolean
  durationSec?: number
  firstLine: number
  lastLine: number
}

export interface BuildLogAnalysis {
  /** Phase per input line index, same length as the input. */
  linePhases: LogPhase[]
  steps: BuildStep[]
  counts: Record<LogPhase, number>
  /** Sum of step durations per phase; parallel steps overlap, so a hint. */
  durations: Record<LogPhase, number>
  truncated: boolean
}

const DEFAULT_MAX_LINES = 50_000

function maxLinesFromEnv(raw: unknown): number {
  const n = typeof raw === 'string' ? Number(raw) : NaN
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : DEFAULT_MAX_LINES
}

export const LOG_PHASE_MAX_LINES = maxLinesFromEnv(
  import.meta.env.VITE_LOG_PHASE_MAX_LINES,
)

const INSTALL_CMD: RegExp[] = [
  /\bnpm\s+(ci|install|i|add)\b/,
  /\bpnpm\s+(install|i|add)\b/,
  /\byarn(\s+(install|add)\b|\s*$|\s*&&|\s*;)/,
  /\bbun\s+(install|i|add)\b/,
  /\bpip3?\s+install\b/,
  /\bpython3?\s+-m\s+pip\s+install\b/,
  /\bpoetry\s+install\b/,
  /\buv\s+(pip\s+install|sync)\b/,
  /\bgo\s+(mod\s+(download|tidy|vendor)|get)\b/,
  /\bbundle\s+install\b/,
  /\bcomposer\s+(install|update|require)\b/,
  /\bcargo\s+(fetch|install)\b/,
  /\bapt(-get)?\s+(install|update|upgrade)\b/,
  /\bapk\s+(add|update|upgrade)\b/,
]

const BUILD_CMD: RegExp[] = [
  /\b(npm|pnpm|yarn|bun)\s+(run\s+)?(build|test|lint|compile)\b/,
  /\b(npm|pnpm|yarn|bun)\s+run\b/,
  /\bgo\s+(build|test|vet|generate)\b/,
  /\bcargo\s+(build|test|check)\b/,
  /\b(vite|next|nuxt|webpack|rollup|esbuild|tsc|astro)\s+build\b/,
  /\btsc\b/,
  /\bmake\b/,
  /\b(mvn|gradle|gradlew|dotnet\s+(build|publish)|mix\s+compile)\b/,
  /\b(pytest|python3?\s+setup\.py|rake|cmake)\b/,
]

const PUSH_TEXT: RegExp[] = [
  /^exporting\s+(to|layers|manifest|config)\b/i,
  /^pushing\b/i,
  /^naming to\b/i,
  /^writing image\b/i,
  /\bdocker\s+push\b/,
  /^The push refers to\b/i,
  /^[0-9a-f]{12}: (Pushed|Layer already exists|Preparing)\b/,
]

// Output signatures that identify an install even without the command line.
const INSTALL_OUTPUT: RegExp[] = [
  /^added \d+ packages?\b/,
  /^Successfully installed\b/,
  /^Collecting \S+/,
  /^go: downloading\b/,
  /^Fetching gem metadata\b/,
  /^Progress: resolved \d+/,
  /^Get:\d+ https?:\/\//,
  /^Lockfile is up to date\b/,
]

const BUILD_OUTPUT: RegExp[] = [/^Compiling \S+/, /^Finished (dev|release)\b/]

function matchesAny(patterns: RegExp[], text: string): boolean {
  return patterns.some((p) => p.test(text))
}

function classifyCommand(cmd: string): LogPhase {
  const build = matchesAny(BUILD_CMD, cmd)
  const install = matchesAny(INSTALL_CMD, cmd)
  if (build) return 'build'
  if (install) return 'install'
  return 'other'
}

// Loose text (no marker): only anchored command prompts and known output
// signatures count, so prose mentioning "npm install" stays `other`.
function classifyLooseText(text: string): LogPhase {
  if (matchesAny(PUSH_TEXT, text)) return 'push'
  const prompt = /^\s*[$+>]\s+(.*)$/.exec(text)
  if (prompt?.[1]) {
    const fromPrompt = classifyCommand(prompt[1])
    if (fromPrompt !== 'other') return fromPrompt
  }
  if (matchesAny(INSTALL_OUTPUT, text)) return 'install'
  if (matchesAny(BUILD_OUTPUT, text)) return 'build'
  return 'other'
}

function classifyStepLabel(label: string): LogPhase {
  if (matchesAny(PUSH_TEXT, label)) return 'push'
  const run = /^RUN\s+(.*)$/i.exec(label)
  if (run?.[1] !== undefined) {
    const phase = classifyCommand(run[1])
    return phase === 'other' ? 'build' : phase
  }
  return 'other'
}

const HEADER_BRACKET = /^#(\d+)\s+(\[[^\]]*\])\s*(.*)$/
const HEADER_PLAIN = /^#(\d+)\s+(\S.*)$/
const DONE = /^#(\d+)\s+DONE\s+(\d+(?:\.\d+)?)s\b/
const CACHED = /^#(\d+)\s+CACHED\b/
const LEGACY_STEP = /^Step \d+\/\d+\s*:\s*(.*)$/
// Plain text after "#N " that belongs to an existing step, never a new one.
const NOISE = /^(DONE|CACHED|ERROR|CANCELED|\d|sha256:|extracting\s+sha256)/

function emptyCounts(): Record<LogPhase, number> {
  return { install: 0, build: 0, push: 0, other: 0 }
}

/** Never throws: malformed or non-string lines are treated as `other`. */
export function analyzeBuildLog(
  lines: readonly LogLine[],
  maxLines: number = LOG_PHASE_MAX_LINES,
): BuildLogAnalysis {
  const linePhases: LogPhase[] = new Array<LogPhase>(lines.length).fill('other')
  const byId = new Map<string, BuildStep>()
  const steps: BuildStep[] = []
  const limit = Math.max(0, Math.min(lines.length, maxLines))
  let legacy: LogPhase | null = null

  for (let i = 0; i < limit; i += 1) {
    const raw = lines[i]?.line
    if (typeof raw !== 'string') continue
    const text = stripAnsiCodes(raw).trim()

    const bk = text.startsWith('#') ? /^#(\d+)\s/.exec(text + ' ') : null
    if (bk?.[1]) {
      legacy = null
      const id = bk[1]
      let step = byId.get(id)
      if (!step) {
        const bracket = HEADER_BRACKET.exec(text)
        const plain = HEADER_PLAIN.exec(text)
        let label = ''
        if (bracket?.[2]) {
          label = bracket[3] || bracket[2]
        } else if (plain?.[2] && !NOISE.test(plain[2])) {
          label = plain[2]
        }
        if (label) {
          step = {
            id,
            label,
            phase: classifyStepLabel(label),
            cached: false,
            firstLine: i,
            lastLine: i,
          }
          byId.set(id, step)
          steps.push(step)
        }
      }
      if (step) {
        step.lastLine = i
        linePhases[i] = step.phase
        const done = DONE.exec(text)
        if (done?.[2]) {
          const sec = Number(done[2])
          if (Number.isFinite(sec)) step.durationSec = sec
        } else if (CACHED.test(text)) {
          step.cached = true
        }
      }
      continue
    }

    const legacyStep = LEGACY_STEP.exec(text)
    if (legacyStep) {
      legacy = classifyStepLabel(legacyStep[1] ?? '')
      linePhases[i] = legacy
      continue
    }
    if (/^Successfully (built|tagged)\b/.test(text)) legacy = null
    const own = classifyLooseText(text)
    linePhases[i] = own !== 'other' ? own : (legacy ?? 'other')
  }

  const counts = emptyCounts()
  for (const p of linePhases) counts[p] += 1
  const durations = emptyCounts()
  for (const s of steps) {
    if (!s.cached && s.durationSec !== undefined)
      durations[s.phase] += s.durationSec
  }
  return {
    linePhases,
    steps,
    counts,
    durations,
    truncated: lines.length > limit,
  }
}

/** True when the log has anything worth offering phase navigation for. */
export function hasRecognisedPhases(a: BuildLogAnalysis): boolean {
  return a.counts.install + a.counts.build + a.counts.push > 0
}

export function formatStepDuration(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) return ''
  if (sec < 60) return `${sec.toFixed(1)}s`
  const m = Math.floor(sec / 60)
  const s = Math.round(sec - m * 60)
  return `${m}m ${s}s`
}
