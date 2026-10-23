import { describe, expect, it } from 'vitest'
import { deriveShellStatus, type ShellStatusInput } from './shellStatus'

const base: ShellStatusInput = {
  level: 'ok',
  updateAvailable: false,
  upgrading: false,
  restarting: false,
}

describe('deriveShellStatus', () => {
  const cases: [string, Partial<ShellStatusInput>, string][] = [
    ['all good', {}, 'healthy'],
    ['update available', { updateAvailable: true }, 'update'],
    ['warning', { level: 'warning' }, 'degraded'],
    ['critical', { level: 'critical' }, 'incident'],
    [
      'critical beats update',
      { level: 'critical', updateAvailable: true },
      'incident',
    ],
    ['loading', { level: null }, 'unknown'],
    [
      'upgrading beats critical',
      { upgrading: true, level: 'critical' },
      'busy',
    ],
    ['restarting', { restarting: true }, 'busy'],
  ]
  it.each(cases)('%s', (_name, patch, want) => {
    expect(deriveShellStatus({ ...base, ...patch })).toBe(want)
  })
})
