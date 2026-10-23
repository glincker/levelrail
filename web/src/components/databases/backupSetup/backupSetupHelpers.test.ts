import { describe, expect, it } from 'vitest'
import {
  canSubmitStep,
  drillState,
  isValidCron,
  parseRetain,
  presetToCron,
} from './backupSetupHelpers'

describe('backupSetupHelpers', () => {
  it('maps presets to cron', () => {
    expect(presetToCron('daily', '')).toBe('0 3 * * *')
    expect(presetToCron('weekly', '')).toBe('0 3 * * 0')
    expect(presetToCron('custom', ' */15 * * * * ')).toBe('*/15 * * * *')
  })

  it('validates cron field count', () => {
    expect(isValidCron('0 3 * * *')).toBe(true)
    expect(isValidCron('0 3 *')).toBe(false)
    expect(isValidCron('')).toBe(false)
  })

  it('parses retention', () => {
    expect(parseRetain('7')).toBe(7)
    expect(parseRetain('0')).toBeNull()
    expect(parseRetain('1.5')).toBeNull()
    expect(parseRetain('x')).toBeNull()
  })

  it('derives drill state', () => {
    expect(drillState(undefined)).toBe('none')
    expect(drillState({ id: '1', at: '', status: 'passed' })).toBe('passed')
    expect(drillState({ id: '1', at: '', status: 'failed' })).toBe('failed')
    expect(drillState({ id: '1', at: '', status: 'running' })).toBe('running')
  })

  it('gates each step', () => {
    const v = { targetId: 't', cron: '0 3 * * *', retain: '7' }
    expect(canSubmitStep('target', { ...v, targetId: '' })).toBe(false)
    expect(canSubmitStep('schedule', { ...v, cron: 'bad' })).toBe(false)
    expect(canSubmitStep('retention', { ...v, retain: '0' })).toBe(false)
    expect(canSubmitStep('retention', v)).toBe(true)
  })
})
