import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { ANSI_TEXT_CLASS, ansiColorVar } from './ansiPalette'
import { stripAnsiCodes } from './ansi'

const tokensCss = readFileSync(
  path.resolve(import.meta.dirname, '../styles/glinr-tokens.css'),
  'utf8',
)

describe('ansiPalette', () => {
  it('maps normal and bright foreground codes to tokens', () => {
    expect(ansiColorVar(31)).toBe('--glinr-code-ansi-red')
    expect(ansiColorVar(97)).toBe('--glinr-code-ansi-bright-white')
    expect(ansiColorVar(1)).toBeNull()
    expect(ansiColorVar(38)).toBeNull()
  })

  it('points every mapped name at a generated token', () => {
    for (const name of Object.keys(ANSI_TEXT_CLASS)) {
      expect(tokensCss).toContain(`--glinr-code-ansi-${name}:`)
    }
    expect(Object.keys(ANSI_TEXT_CLASS)).toHaveLength(16)
  })

  it('still strips escapes from rendered text', () => {
    expect(stripAnsiCodes('\u001b[31mfail\u001b[0m')).toBe('fail')
  })
})
