import { describe, expect, it } from 'vitest'
import type { RuntimeSetting } from '../types/runtimeSettings'
import {
  filterSettings,
  groupByCategory,
  listFromText,
  needsConfirm,
  parseCategory,
  splitMatches,
  settingItemId,
} from './runtimeSettings'

function setting(over: Partial<RuntimeSetting>): RuntimeSetting {
  return {
    key: 'security.x',
    category: 'security',
    kind: 'bool',
    title: 'X',
    description: '',
    default: true,
    value: true,
    source: 'default',
    env: '',
    apply: 'live',
    risk: 'normal',
    onboarding: false,
    pending_restart: false,
    ...over,
  }
}

describe('runtimeSettings helpers', () => {
  it('groups in category order and drops empty groups', () => {
    const groups = groupByCategory([
      setting({ key: 'a', category: 'deploys' }),
      setting({ key: 'b', category: 'security' }),
    ])
    expect(groups.map((g) => g.category)).toEqual(['security', 'deploys'])
  })

  it('parses one list entry per line', () => {
    expect(listFromText(' /srv \n\n/data\n')).toEqual(['/srv', '/data'])
  })

  it('confirms only when weakening', () => {
    const weak = setting({ risk: 'weakens_security' })
    expect(needsConfirm(weak, false)).toBe(true)
    expect(needsConfirm(weak, true)).toBe(false)
    expect(needsConfirm(setting({}), false)).toBe(false)
  })

  it('derives a camel-cased item id after the category prefix', () => {
    expect(settingItemId('security.build_network')).toBe('buildNetwork')
    expect(settingItemId('api.rate_limit_read_rpm')).toBe('rateLimitReadRpm')
  })

  it('filters by title, key and env, and by changed-from-default', () => {
    const rows = [
      setting({ key: 'api.rate_limit_read_rpm', env: 'APP_RATE_READ' }),
      setting({ key: 'security.other', source: 'saved' }),
    ]
    const text = (s: RuntimeSetting) => ({
      title: s.key === 'security.other' ? 'Other' : 'Reads',
      description: '',
    })
    const all = { query: '', changedOnly: false }
    expect(filterSettings(rows, { ...all, query: 'RATE' }, text)).toHaveLength(
      1,
    )
    expect(
      filterSettings(rows, { ...all, query: 'app_rate' }, text),
    ).toHaveLength(1)
    expect(filterSettings(rows, { ...all, query: 'oth' }, text)).toHaveLength(1)
    expect(
      filterSettings(rows, { ...all, changedOnly: true }, text),
    ).toHaveLength(1)
    expect(filterSettings(rows, { ...all, query: 'zzz' }, text)).toHaveLength(0)
  })
})

describe('category and highlight helpers', () => {
  const text = (s: RuntimeSetting) => ({ title: s.title, description: '' })

  it('filters by category together with the query', () => {
    const list = [
      setting({ key: 'security.a', title: 'Alpha' }),
      setting({ key: 'networking.a', category: 'networking', title: 'Alpha' }),
    ]
    const out = filterSettings(
      list,
      { query: 'alpha', changedOnly: false, category: 'networking' },
      text,
    )
    expect(out.map((s) => s.key)).toEqual(['networking.a'])
  })

  it('parses only known categories', () => {
    expect(parseCategory('deploys')).toBe('deploys')
    expect(parseCategory('nope')).toBeUndefined()
    expect(parseCategory(undefined)).toBeUndefined()
  })

  it('splits text around every case-insensitive match', () => {
    expect(splitMatches('Trust proxy TRUST', 'trust')).toEqual([
      { text: 'Trust', match: true },
      { text: ' proxy ', match: false },
      { text: 'TRUST', match: true },
    ])
    expect(splitMatches('abc', '  ')).toEqual([{ text: 'abc', match: false }])
  })
})
