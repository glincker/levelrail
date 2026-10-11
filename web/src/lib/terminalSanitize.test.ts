import { describe, expect, it } from 'vitest'
import {
  EMPTY_SANITIZE_STATE,
  sanitizeTerminalOutput,
  sanitizeTerminalText,
} from './terminalSanitize'

describe('sanitizeTerminalText', () => {
  const cases: Array<{ name: string; input: string; want: string }> = [
    {
      name: 'plain text untouched',
      input: 'hello world\r\nline 2\t\b',
      want: 'hello world\r\nline 2\t\b',
    },
    {
      name: 'SGR color preserved',
      input: '\x1b[31mred\x1b[0m \x1b[1;38;5;12mx\x1b[m',
      want: '\x1b[31mred\x1b[0m \x1b[1;38;5;12mx\x1b[m',
    },
    { name: 'OSC title with BEL', input: 'a\x1b]0;pwned\x07b', want: 'ab' },
    { name: 'OSC title with ST', input: 'a\x1b]2;pwned\x1b\\b', want: 'ab' },
    {
      name: 'OSC 8 hyperlink',
      input: '\x1b]8;;http://evil\x07click\x1b]8;;\x07',
      want: 'click',
    },
    { name: 'DCS string', input: 'a\x1bP1$r0m\x1b\\b', want: 'ab' },
    { name: 'APC string', input: 'a\x1b_payload\x1b\\b', want: 'ab' },
    {
      name: 'PM and SOS strings',
      input: 'a\x1b^x\x1b\\b\x1bXy\x1b\\c',
      want: 'abc',
    },
    {
      name: 'fake DSR cursor position request',
      input: 'a\x1b[6nb',
      want: 'ab',
    },
    { name: 'DSR status request', input: 'a\x1b[5nb', want: 'ab' },
    { name: 'DA and secondary DA', input: 'a\x1b[cb\x1b[>cc', want: 'abc' },
    { name: 'XTWINOPS', input: 'a\x1b[18tb\x1b[21tc', want: 'abc' },
    {
      name: 'other C0 controls dropped',
      input: 'a\x00\x07\x0b\x0c\x0e\x7fb',
      want: 'ab',
    },
    { name: 'C1 introducers dropped', input: 'a\x9b\x9db', want: 'ab' },
    { name: 'RIS dropped', input: 'a\x1bcb', want: 'ab' },
    { name: 'charset select dropped', input: 'a\x1b(Bb', want: 'ab' },
  ]

  it.each(cases)('$name', ({ input, want }) => {
    expect(sanitizeTerminalText(input)).toBe(want)
  })
})

describe('sanitizeTerminalOutput chunking', () => {
  const splits: Array<{ name: string; chunks: string[]; want: string }> = [
    { name: 'DSR split after ESC', chunks: ['a\x1b', '[6nb'], want: 'ab' },
    {
      name: 'DSR split inside params',
      chunks: ['a\x1b[', '6', 'nb'],
      want: 'ab',
    },
    {
      name: 'SGR split keeps color',
      chunks: ['\x1b[3', '1mred'],
      want: '\x1b[31mred',
    },
    {
      name: 'OSC split before terminator',
      chunks: ['a\x1b]0;ti', 'tle\x07b'],
      want: 'ab',
    },
    {
      name: 'OSC split inside ST',
      chunks: ['a\x1b]0;t\x1b', '\\b'],
      want: 'ab',
    },
    {
      name: 'DCS split across three chunks',
      chunks: ['a\x1bP', 'data', '\x1b\\b'],
      want: 'ab',
    },
    {
      name: 'text after unterminated OSC stays hidden',
      chunks: ['a\x1b]0;x', 'still hidden'],
      want: 'a',
    },
  ]

  it.each(splits)('$name', ({ chunks, want }) => {
    let state = EMPTY_SANITIZE_STATE
    let out = ''
    for (const chunk of chunks) {
      const res = sanitizeTerminalOutput(chunk, state)
      out += res.text
      state = res.state
    }
    expect(out).toBe(want)
  })
})
