import { describe, expect, it } from 'vitest'
import {
  DEFAULT_ANSI_STYLE,
  ansiClassName,
  ansiPlainText,
  extendAnsiStates,
  newAnsiStateCache,
  parseAnsiLine,
} from './ansiRender'
import { stripAnsiCodes } from './ansi'
import { highlightStyled } from './logHighlight'

const E = '\u001b'

describe('parseAnsiLine', () => {
  it('colours npm warn and error prefixes', () => {
    const line = `npm ${E}[33mwarn${E}[39m deprecated inflight@1.0.6`
    const { segments, end } = parseAnsiLine(line)
    expect(segments.map((s) => s.text)).toEqual([
      'npm ',
      'warn',
      ' deprecated inflight@1.0.6',
    ])
    expect(segments[1]?.style.fg).toBe('yellow')
    expect(end).toEqual(DEFAULT_ANSI_STYLE)
  })

  it('handles next build output with bold, dim and bright colours', () => {
    const line = `${E}[1m${E}[32m✓${E}[39m${E}[22m Compiled ${E}[2m(1.2s)${E}[22m ${E}[90mroute${E}[0m`
    const { segments } = parseAnsiLine(line)
    expect(ansiPlainText(line)).toBe('✓ Compiled (1.2s) route')
    expect(segments[0]?.style).toMatchObject({ fg: 'green', bold: true })
    expect(segments.find((s) => s.text === '(1.2s)')?.style.dim).toBe(true)
    expect(segments[segments.length - 1]?.style.fg).toBe('bright-black')
  })

  it('maps docker 256 colour and truecolor to the nearest token', () => {
    const a = parseAnsiLine(`${E}[38;5;1mred${E}[0m ${E}[38;5;196mhot${E}[0m`)
    expect(a.segments[0]?.style.fg).toBe('red')
    expect(a.segments[2]?.style.fg).toMatch(/red/)
    const b = parseAnsiLine(`${E}[38;2;0;200;0mgo${E}[0m`)
    expect(b.segments[0]?.style.fg).toMatch(/green/)
  })

  it('ignores background colours and underlines go through', () => {
    const { segments } = parseAnsiLine(`${E}[44;4mx${E}[0m`)
    expect(segments[0]?.style).toMatchObject({ fg: null, underline: true })
  })

  it('strips cursor movement and OSC titles from go test style output', () => {
    const line = `${E}[2K${E}[1G${E}]0;title\u0007ok  \tgithub.com/x/y\t0.4s${E}[?25h`
    expect(ansiPlainText(line)).toBe('ok  \tgithub.com/x/y\t0.4s')
  })

  it('never emits markup: tags stay literal text', () => {
    const { segments } = parseAnsiLine(`${E}[31m<img src=x onerror=1>${E}[0m`)
    expect(segments[0]?.text).toBe('<img src=x onerror=1>')
  })

  it('agrees with stripAnsiCodes on common sequences', () => {
    const line = `${E}[1;31merror${E}[0m ${E}[2Kdone`
    expect(ansiPlainText(line)).toBe(stripAnsiCodes(line))
  })

  it('caps pathological input without throwing', () => {
    const many = `${E}[31ma${E}[32mb`.repeat(5000)
    const { segments } = parseAnsiLine(many)
    expect(segments.length).toBeLessThanOrEqual(200)
    const unterminated = `${E}]0;${'x'.repeat(100000)}`
    expect(() => parseAnsiLine(unterminated)).not.toThrow()
    expect(parseAnsiLine(`${E}[${'9;'.repeat(50000)}m`).segments).toEqual([])
  })
})

describe('stream state', () => {
  const lines = [
    { id: 1, line: `${E}[31mfailed step` },
    { id: 2, line: 'still red' },
    { id: 3, line: `${E}[0mplain` },
    { id: 4, line: 'plain too' },
  ]

  it('carries colour across lines until reset', () => {
    const starts = extendAnsiStates(newAnsiStateCache(), lines)
    expect(starts.get(2)?.fg).toBe('red')
    expect(starts.get(3)?.fg).toBe('red')
    expect(starts.has(4)).toBe(false)
    const row = parseAnsiLine(lines[1]?.line ?? '', starts.get(2))
    expect(ansiClassName(row.segments[0]?.style ?? DEFAULT_ANSI_STYLE)).toBe(
      'text-glinr-code-ansi-red',
    )
  })

  it('extends incrementally and survives a dropped head', () => {
    const cache = newAnsiStateCache()
    extendAnsiStates(cache, lines.slice(0, 2))
    const starts = extendAnsiStates(cache, lines)
    expect(starts.get(3)?.fg).toBe('red')
    const reset = extendAnsiStates(cache, [{ id: 9, line: 'new' }])
    expect(reset.size).toBe(0)
  })
})

describe('highlightStyled', () => {
  it('matches across segment boundaries on plain text', () => {
    const { segments } = parseAnsiLine(`${E}[31mfa${E}[32mil${E}[0m ok`)
    const pieces = highlightStyled(segments, 'fail')
    expect(pieces.filter((p) => p.match).map((p) => p.text)).toEqual([
      'fa',
      'il',
    ])
    expect(pieces.map((p) => p.text).join('')).toBe('fail ok')
  })
})
