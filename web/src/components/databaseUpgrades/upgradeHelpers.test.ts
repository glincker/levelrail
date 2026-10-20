import { describe, expect, it } from 'vitest'
import type { UpgradePolicy } from '../../types/databaseUpgrades'
import {
  draftToInput,
  hoursToSeconds,
  policyToDraft,
  presetForCron,
  secondsToHours,
  supportVariant,
  toggleChannel,
} from './upgradeHelpers'

const policy: UpgradePolicy = {
  auto_upgrade: 'patch',
  window_cron: '0 3 * * 0',
  window_duration_seconds: 5400,
  window_timezone: 'Europe/Berlin',
  backup_before: true,
  verify_after: true,
  revert_on_failure: false,
  notify: ['ch1'],
  inherited: false,
}

describe('upgradeHelpers', () => {
  it.each([
    ['0 3 * * *', 'daily'],
    ['0 3 * * 0', 'sundays'],
    [' 0 3 * * 0 ', 'sundays'],
    ['*/5 * * * *', 'custom'],
  ] as const)('maps cron %s to preset %s', (cron, preset) => {
    expect(presetForCron(cron)).toBe(preset)
  })

  it('converts between hours and seconds', () => {
    expect(hoursToSeconds(1.5)).toBe(5400)
    expect(secondsToHours(5400)).toBe(1.5)
  })

  it('round trips a policy through the draft', () => {
    const input = draftToInput(policyToDraft(policy))
    expect(input).toEqual({
      auto_upgrade: 'patch',
      window_cron: '0 3 * * 0',
      window_duration_seconds: 5400,
      window_timezone: 'Europe/Berlin',
      verify_after: true,
      revert_on_failure: false,
      notify: ['ch1'],
    })
  })

  it('maps support to badge variants', () => {
    expect(supportVariant('supported')).toBe('success')
    expect(supportVariant('eol_soon')).toBe('warning')
    expect(supportVariant('eol')).toBe('destructive')
    expect(supportVariant('unknown')).toBe('muted')
  })

  it('toggles channel ids', () => {
    expect(toggleChannel(['a'], 'b')).toEqual(['a', 'b'])
    expect(toggleChannel(['a', 'b'], 'a')).toEqual(['b'])
  })
})
