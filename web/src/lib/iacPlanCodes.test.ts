import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import en from '../locales/en/settings.json'
import { IAC_CODE_KEYS } from './iacPlanCodes'

// The published list is the generated errors page, so a new server code fails here until mapped.
const PAGE = join(__dirname, '..', '..', '..', 'docs', 'iac', 'errors.md')
const published = [
  ...readFileSync(PAGE, 'utf8').matchAll(/^\| `(\w+)` \|/gm),
].flatMap((m) => (m[1] ? [m[1]] : []))

describe('iac plan code map', () => {
  it('reads a plausible published list', () => {
    expect(published.length).toBeGreaterThan(40)
  })

  it.each(published)('maps %s to a message and fix', (code) => {
    expect(IAC_CODE_KEYS[code]).toBe(`iacPlan.codes.${code}`)
    const entry = (
      en.iacPlan.codes as Record<string, { message?: string; fix?: string }>
    )[code]
    expect(entry?.message).toBeTruthy()
    expect(entry?.fix).toBeTruthy()
  })

  it('has no mapped code the server does not publish', () => {
    expect(Object.keys(IAC_CODE_KEYS).sort()).toEqual([...published].sort())
  })
})
