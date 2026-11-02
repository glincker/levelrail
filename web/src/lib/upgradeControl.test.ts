import { describe, expect, it } from 'vitest'
import {
  deriveActions,
  downloadPercent,
  isStalled,
  type ActionInput,
  type PrimaryAction,
} from './upgradeControl'

const idle: ActionInput = {
  running: false,
  stalled: false,
  failed: false,
  updateAvailable: false,
  canApply: true,
  step: '',
}

describe('deriveActions', () => {
  const cases: Array<[string, Partial<ActionInput>, PrimaryAction]> = [
    ['up to date', {}, 'check'],
    ['update available', { updateAvailable: true }, 'update'],
    ['update blocked', { updateAvailable: true, canApply: false }, 'check'],
    ['downloading', { running: true, step: 'download' }, 'cancel'],
    [
      'stalled download',
      { running: true, stalled: true, step: 'download' },
      'retry',
    ],
    ['past swap', { running: true, step: 'swap' }, 'check'],
    ['failed', { failed: true }, 'retry'],
  ]
  it.each(cases)('%s', (_n, over, want) => {
    expect(deriveActions({ ...idle, ...over }).primary).toBe(want)
  })

  it('stuck run offers cancel then retry, server can_cancel wins', () => {
    const a = deriveActions({
      ...idle,
      running: true,
      stalled: true,
      step: 'swap',
      canCancel: true,
    })
    expect(a.showCancel).toBe(true)
    expect(a.showRetry).toBe(true)
    expect(
      deriveActions({ ...idle, running: true, stalled: true, canCancel: false })
        .showRetry,
    ).toBe(false)
  })

  it('flags cancelling and honors can_retry on a failed attempt', () => {
    expect(
      deriveActions({ ...idle, running: true, cancelRequested: true })
        .cancelling,
    ).toBe(true)
    expect(
      deriveActions({ ...idle, failed: true, canRetry: false }).showRetry,
    ).toBe(false)
  })
})

describe('isStalled', () => {
  const now = 1_000_000
  it('trusts the server flag', () => {
    expect(isStalled({ serverStalled: true, lastChangeAt: now, now })).toBe(
      true,
    )
    expect(isStalled({ serverStalled: false, lastChangeAt: 0, now })).toBe(
      false,
    )
  })
  it('falls back to the client threshold', () => {
    expect(
      isStalled({ lastChangeAt: now - 59_000, now, thresholdMs: 60_000 }),
    ).toBe(false)
    expect(
      isStalled({ lastChangeAt: now - 60_000, now, thresholdMs: 60_000 }),
    ).toBe(true)
  })
  it('uses the newer server progress mark', () => {
    const at = new Date(now - 5_000).toISOString()
    expect(
      isStalled({
        lastProgressAt: at,
        lastChangeAt: 0,
        now,
        thresholdMs: 60_000,
      }),
    ).toBe(false)
  })
})

describe('downloadPercent', () => {
  it('is null without a total', () => {
    expect(downloadPercent(10, 0)).toBeNull()
    expect(downloadPercent(10, undefined)).toBeNull()
  })
  it('clamps and rounds', () => {
    expect(downloadPercent(50, 200)).toBe(25)
    expect(downloadPercent(300, 200)).toBe(100)
  })
})
