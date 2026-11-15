// Parses SGR colour escapes into styled text segments (plain data, no HTML) and drops every
// other control sequence. Colour persists across lines until reset, so callers pass a start state.
import { ANSI_TEXT_CLASS } from './ansiPalette'
import { stripAnsiCodes } from './ansi'

export interface AnsiStyle {
  fg: string | null
  bold: boolean
  dim: boolean
  underline: boolean
}

export interface AnsiSegment {
  text: string
  style: AnsiStyle
}

export const DEFAULT_ANSI_STYLE: AnsiStyle = {
  fg: null,
  bold: false,
  dim: false,
  underline: false,
}

function envInt(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : fallback
}

// Lines past either cap render as plain stripped text so hostile input cannot stall the UI.
const MAX_LINE_CHARS = envInt(
  import.meta.env.VITE_LOG_ANSI_MAX_LINE_CHARS,
  8000,
)
const MAX_SEGMENTS = envInt(import.meta.env.VITE_LOG_ANSI_MAX_SEGMENTS, 200)
const MAX_SEQ_CHARS = 64

const ESC = '\u001b'
const CSI8 = '\u009b'
const NAMES = [
  'black',
  'red',
  'green',
  'yellow',
  'blue',
  'magenta',
  'cyan',
  'white',
] as const

export function isDefaultStyle(s: AnsiStyle): boolean {
  return s.fg === null && !s.bold && !s.dim && !s.underline
}

function nameFor(index: number, bright: boolean): string {
  const base = NAMES[index] ?? 'white'
  return bright ? `bright-${base}` : base
}

// Nearest of the 8 hues by dominant channel; greys fall to black or white.
function rgbToName(r: number, g: number, b: number): string {
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const bright = max >= 200
  if (max - min < 40) {
    const lum = (r + g + b) / 3
    if (lum < 70) return 'bright-black'
    return lum > 170 ? 'bright-white' : 'white'
  }
  const hi = (v: number) => v >= max - 55
  if (hi(r) && hi(g) && hi(b)) return nameFor(7, bright)
  if (hi(r) && hi(g)) return nameFor(3, bright)
  if (hi(r) && hi(b)) return nameFor(5, bright)
  if (hi(g) && hi(b)) return nameFor(6, bright)
  if (hi(r)) return nameFor(1, bright)
  if (hi(g)) return nameFor(2, bright)
  return nameFor(4, bright)
}

function color256(n: number): string | null {
  if (!Number.isInteger(n) || n < 0 || n > 255) return null
  if (n < 8) return nameFor(n, false)
  if (n < 16) return nameFor(n - 8, true)
  if (n >= 232) return rgbToName(...gray(n))
  const c = n - 16
  const lv = (v: number) => (v === 0 ? 0 : 55 + v * 40)
  return rgbToName(
    lv(Math.floor(c / 36)),
    lv(Math.floor((c % 36) / 6)),
    lv(c % 6),
  )
}

function gray(n: number): [number, number, number] {
  const v = 8 + (n - 232) * 10
  return [v, v, v]
}

function applySgr(prev: AnsiStyle, params: number[]): AnsiStyle {
  const s = { ...prev }
  for (let i = 0; i < params.length; i++) {
    const p = params[i] ?? 0
    if (p === 0) Object.assign(s, DEFAULT_ANSI_STYLE)
    else if (p === 1) s.bold = true
    else if (p === 2) s.dim = true
    else if (p === 4) s.underline = true
    else if (p === 22) {
      s.bold = false
      s.dim = false
    } else if (p === 24) s.underline = false
    else if (p === 39) s.fg = null
    else if (p >= 30 && p <= 37) s.fg = nameFor(p - 30, false)
    else if (p >= 90 && p <= 97) s.fg = nameFor(p - 90, true)
    else if (p === 38 || p === 48) {
      const mode = params[i + 1]
      const skip = mode === 5 ? 2 : mode === 2 ? 4 : 1
      if (p === 38) {
        if (mode === 5) s.fg = color256(params[i + 2] ?? -1)
        else if (mode === 2) {
          s.fg = rgbToName(
            params[i + 2] ?? 0,
            params[i + 3] ?? 0,
            params[i + 4] ?? 0,
          )
        }
      }
      i += skip
    }
  }
  return s
}

function sameStyle(a: AnsiStyle, b: AnsiStyle): boolean {
  return (
    a.fg === b.fg &&
    a.bold === b.bold &&
    a.dim === b.dim &&
    a.underline === b.underline
  )
}

// Returns the index after the sequence starting at i (an ESC or CSI8), and the SGR params if any.
function readSequence(
  line: string,
  i: number,
): { next: number; sgr: number[] | null } {
  const first = line[i]
  let j = i + 1
  if (first === ESC) {
    const kind = line[j]
    if (kind === ']') {
      // OSC ends at BEL or ESC \; give up at the cap and drop what was scanned.
      const stop = Math.min(line.length, j + MAX_SEQ_CHARS * 8)
      for (let k = j + 1; k < stop; k++) {
        if (line[k] === '\u0007') return { next: k + 1, sgr: null }
        if (line[k] === ESC && line[k + 1] === '\\') {
          return { next: k + 2, sgr: null }
        }
      }
      return { next: stop, sgr: null }
    }
    if (kind !== '[') {
      // Two-byte escapes, or charset selects like ESC ( B that take one more byte.
      const extra = kind !== undefined && '()#%'.includes(kind) ? 2 : 1
      return { next: Math.min(line.length, j + extra), sgr: null }
    }
    j++
  }
  const paramStart = j
  while (j < line.length && j - paramStart < MAX_SEQ_CHARS) {
    const code = line.charCodeAt(j)
    if (code < 0x20 || code > 0x3f) break
    j++
  }
  const params = line.slice(paramStart, j)
  while (
    j < line.length &&
    line.charCodeAt(j) >= 0x20 &&
    line.charCodeAt(j) <= 0x2f
  )
    j++
  const final = line[j]
  if (final === undefined || final.charCodeAt(0) < 0x40) {
    return { next: j, sgr: null }
  }
  if (final !== 'm' || !/^[0-9;:]*$/.test(params))
    return { next: j + 1, sgr: null }
  const nums =
    params === ''
      ? [0]
      : params
          .split(/[;:]/)
          .slice(0, 32)
          .map((v) => Number(v) || 0)
  return { next: j + 1, sgr: nums }
}

export function parseAnsiLine(
  line: string,
  start: AnsiStyle = DEFAULT_ANSI_STYLE,
): { segments: AnsiSegment[]; end: AnsiStyle } {
  if (!line.includes(ESC) && !line.includes(CSI8)) {
    return {
      segments: line === '' ? [] : [{ text: line, style: start }],
      end: start,
    }
  }
  if (line.length > MAX_LINE_CHARS) {
    const text = stripAnsiCodes(line)
    return { segments: text === '' ? [] : [{ text, style: start }], end: start }
  }
  const segments: AnsiSegment[] = []
  let style = start
  let buf = ''
  const flush = () => {
    if (buf === '') return
    const last = segments[segments.length - 1]
    // Past the segment cap, text keeps flowing into the last span instead of adding more.
    if (
      last &&
      (segments.length >= MAX_SEGMENTS || sameStyle(last.style, style))
    ) {
      last.text += buf
    } else segments.push({ text: buf, style })
    buf = ''
  }
  let i = 0
  while (i < line.length) {
    const ch = line[i]
    if (ch !== ESC && ch !== CSI8) {
      buf += ch
      i++
      continue
    }
    const { next, sgr } = readSequence(line, i)
    if (sgr) {
      flush()
      style = applySgr(style, sgr)
    }
    i = next
  }
  flush()
  return { segments, end: style }
}

export function ansiPlainText(line: string): string {
  return parseAnsiLine(line)
    .segments.map((s) => s.text)
    .join('')
}

export function ansiClassName(style: AnsiStyle): string {
  const parts: string[] = []
  if (style.fg !== null) {
    const cls = ANSI_TEXT_CLASS[style.fg]
    if (cls) parts.push(cls)
  }
  if (style.bold) parts.push('font-bold')
  if (style.dim) parts.push('opacity-70')
  if (style.underline) parts.push('underline')
  return parts.join(' ')
}

export interface AnsiStateCache {
  lastId: number | null
  end: AnsiStyle
  starts: Map<number, AnsiStyle>
}

export function newAnsiStateCache(): AnsiStateCache {
  return { lastId: null, end: DEFAULT_ANSI_STYLE, starts: new Map() }
}

// Records the start style of every line that begins coloured. Incremental: only lines after
// the last seen id are scanned, and only lines containing an escape are parsed.
export function extendAnsiStates(
  cache: AnsiStateCache,
  lines: readonly { id: number; line: string }[],
): Map<number, AnsiStyle> {
  let from = 0
  if (cache.lastId !== null) {
    let at = -1
    for (let k = lines.length - 1; k >= 0 && at < 0; k--) {
      if (lines[k]?.id === cache.lastId) at = k
    }
    if (at >= 0) from = at + 1
    else {
      cache.starts.clear()
      cache.end = DEFAULT_ANSI_STYLE
    }
  }
  for (let k = from; k < lines.length; k++) {
    const l = lines[k]
    if (!l) continue
    if (!isDefaultStyle(cache.end)) cache.starts.set(l.id, cache.end)
    if (l.line.includes(ESC) || l.line.includes(CSI8)) {
      cache.end = parseAnsiLine(l.line, cache.end).end
    }
  }
  const last = lines[lines.length - 1]
  if (last) cache.lastId = last.id
  if (cache.starts.size > lines.length * 2 + 64) {
    const keep = new Set(lines.map((l) => l.id))
    for (const id of cache.starts.keys())
      if (!keep.has(id)) cache.starts.delete(id)
  }
  return cache.starts
}
