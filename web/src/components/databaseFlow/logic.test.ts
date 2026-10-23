import { describe, expect, it } from 'vitest'
import {
  backupRetain,
  defaultPreset,
  pickVersion,
  presetResources,
  scheduleCron,
  suggestName,
  summaryKey,
  validate,
  validateName,
  type DbValues,
} from './logic'

const base: DbValues = {
  engine: 'postgres',
  version: '16',
  name: 'postgres',
  backups: true,
  schedule: 'daily',
  preset: 'small',
  nodeId: '',
  projectId: '',
}

describe('suggestName', () => {
  it('uses the engine id when free', () => {
    expect(suggestName('redis', [])).toBe('redis')
  })
  it('numbers past taken names', () => {
    expect(suggestName('redis', ['redis', 'redis-2'])).toBe('redis-3')
  })
  it('is empty without an engine', () => {
    expect(suggestName('', [])).toBe('')
  })
})

describe('validation', () => {
  it('flags empty, malformed and taken names', () => {
    expect(validateName('  ', [])).toBe('required')
    expect(validateName('My DB', [])).toBe('format')
    expect(validateName('main', ['main'])).toBe('taken')
    expect(validateName('main-1', ['main'])).toBeUndefined()
  })
  it('reports a missing version', () => {
    expect(validate({ ...base, version: ' ' }, [])).toEqual({
      version: 'required',
    })
  })
})

describe('engine defaults', () => {
  it('sizes heavy engines up', () => {
    expect(defaultPreset('clickhouse')).toBe('medium')
    expect(defaultPreset('redis')).toBe('small')
  })
  it('prefers the recommended version until the user edits it', () => {
    const list = [
      { version: '17', recommended: false },
      { version: '16', recommended: true },
    ]
    expect(pickVersion('', false, list, '15')).toBe('16')
    expect(pickVersion('17', true, list, '15')).toBe('17')
    expect(pickVersion('', false, [], '15')).toBe('15')
  })
  it('maps presets and schedules to wire values', () => {
    expect(presetResources('medium')).toEqual({
      memory_bytes: 1024 * 1024 * 1024,
      nano_cpus: 1e9,
    })
    expect(scheduleCron('weekly')).toBe('0 3 * * 0')
    expect(backupRetain()).toBeGreaterThan(0)
  })
})

describe('summaryKey', () => {
  it('drops the backup clause when unavailable', () => {
    expect(summaryKey(base, true)).toBe('flow.summary.withBackups')
    expect(summaryKey(base, false)).toBe('flow.summary.noBackups')
    expect(summaryKey({ ...base, backups: false }, true)).toBe(
      'flow.summary.noBackups',
    )
    expect(summaryKey({ ...base, engine: 'memcached' }, true)).toBe(
      'flow.summary.noBackups',
    )
  })
})
