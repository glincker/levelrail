import { describe, expect, it } from 'vitest'
import type { RuntimeSetting } from '../types/runtimeSettings'
import { groupByCategory, listFromText, needsConfirm } from './runtimeSettings'

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
})
