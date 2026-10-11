import { describe, expect, it } from 'vitest'
import { humanizeCondition, humanizeIdentifier } from './conditionCopy'

describe('humanizeIdentifier', () => {
  it.each([
    ['AlreadyRunning', 'Already running'],
    ['Ready', 'Ready'],
    ['TLSReady', 'TLS ready'],
    ['CredentialsNotConfigured', 'Credentials not configured'],
    ['', ''],
  ])('%s -> %s', (input, want) => {
    expect(humanizeIdentifier(input)).toBe(want)
  })
})

describe('humanizeCondition', () => {
  it('never exposes True or Ready: AlreadyRunning as the title', () => {
    const h = humanizeCondition({
      Type: 'Ready',
      Status: 'True',
      Reason: 'AlreadyRunning',
      Message: '',
      LastTransitionTime: '2026-10-01T00:00:00Z',
    })
    expect(h).toEqual({
      outcome: 'ok',
      title: 'Ready',
      detail: 'Already running',
      raw: 'Ready: AlreadyRunning (True)',
    })
  })
})
