import type { CliCommand } from './cliConsoleProtocol'

export const LOCAL_COMMANDS = ['help', 'clear'] as const
const HISTORY_LIMIT = 100

export interface LineState {
  buffer: string
  cursor: number
}

export const EMPTY_LINE: LineState = { buffer: '', cursor: 0 }

export function insertText(s: LineState, text: string): LineState {
  const clean = text.replace(/[\r\n]+/g, ' ')
  return {
    buffer: s.buffer.slice(0, s.cursor) + clean + s.buffer.slice(s.cursor),
    cursor: s.cursor + clean.length,
  }
}

export function backspace(s: LineState): LineState {
  if (s.cursor === 0) return s
  return {
    buffer: s.buffer.slice(0, s.cursor - 1) + s.buffer.slice(s.cursor),
    cursor: s.cursor - 1,
  }
}

export function deleteForward(s: LineState): LineState {
  if (s.cursor >= s.buffer.length) return s
  return {
    buffer: s.buffer.slice(0, s.cursor) + s.buffer.slice(s.cursor + 1),
    cursor: s.cursor,
  }
}

export function moveCursor(s: LineState, to: number): LineState {
  return { ...s, cursor: Math.max(0, Math.min(s.buffer.length, to)) }
}

export function pushHistory(history: string[], line: string): string[] {
  const trimmed = line.trim()
  if (trimmed === '' || history[history.length - 1] === trimmed) return history
  return [...history, trimmed].slice(-HISTORY_LIMIT)
}

function commonPrefix(words: string[]): string {
  let prefix = words[0] ?? ''
  for (const w of words) {
    while (!w.startsWith(prefix)) prefix = prefix.slice(0, -1)
  }
  return prefix
}

export interface Completion {
  line: LineState
  candidates: string[]
}

// Completes the word under the cursor against the command tree. Only the
// first word (command) and second word (subcommand) have candidates.
export function complete(s: LineState, tree: CliCommand[]): Completion {
  const head = s.buffer.slice(0, s.cursor)
  const rest = s.buffer.slice(s.cursor)
  const words = head.split(/\s+/)
  const index = words.length - 1
  const word = words[index] ?? ''
  let pool: string[] = []
  if (index === 0) {
    pool = [...tree.map((c) => c.name), ...LOCAL_COMMANDS]
  } else if (index === 1) {
    const parent = words[0] ?? ''
    pool =
      parent === 'help'
        ? tree.map((c) => c.name)
        : (tree.find((c) => c.name === parent)?.subcommands ?? [])
  }
  const matches = pool.filter((p) => p.startsWith(word)).sort()
  if (matches.length === 0) return { line: s, candidates: [] }
  const base = head.slice(0, head.length - word.length)
  const only = matches.length === 1
  const filled = only ? `${matches[0] ?? ''} ` : commonPrefix(matches)
  return {
    line: {
      buffer: base + filled + rest,
      cursor: base.length + filled.length,
    },
    candidates: only || filled.length > word.length ? [] : matches,
  }
}
