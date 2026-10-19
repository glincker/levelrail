import { describe, expect, it } from 'vitest'
import type { FwCondition, FwRule } from '../../queries/firewallEdge'
import type { RuleDraft } from '../rules/ruleModel'
import {
  conditionText,
  draftForValue,
  draftFromRule,
  isIpListRule,
  isIpOrCidr,
  mapServerErrors,
  movedOrder,
  ruleFromDraft,
  ruleJson,
  validateDraft,
  type FwMessages,
} from './firewallRuleModel'

const m: FwMessages = {
  nameRequired: 'name',
  noteTooLong: 'note',
  actionRequired: 'action',
  rateCount: 'count',
  rateWindow: 'window',
  paramName: 'paramName',
  valueRequired: 'value',
  ipInvalid: 'ip',
  methodInvalid: 'method',
  countryInvalid: 'country',
  pathSlash: 'slash',
  wildcards: 'wildcards',
}

function draft(
  over: Partial<RuleDraft> = {},
  c: Partial<RuleDraft['conditions'][number]> = {},
): RuleDraft {
  return {
    name: 'Rule',
    note: '',
    action: 'deny',
    params: {},
    conditions: [
      { field: 'path', op: 'starts_with', name: '', value: '/admin', ...c },
    ],
    ...over,
  }
}

describe('validateDraft', () => {
  const cases: [string, RuleDraft, Record<string, string>][] = [
    ['valid', draft(), {}],
    ['no name', draft({ name: ' ' }), { name: 'name' }],
    ['long name', draft({ name: 'x'.repeat(81) }), { name: 'name' }],
    ['long note', draft({ note: 'x'.repeat(201) }), { note: 'note' }],
    ['no action', draft({ action: '' }), { action: 'action' }],
    [
      'rate missing',
      draft({ action: 'rate_limit' }),
      { 'params.rate_count': 'count', 'params.rate_window_seconds': 'window' },
    ],
    [
      'rate ok',
      draft({
        action: 'rate_limit',
        params: { rate_count: '10', rate_window_seconds: '60' },
      }),
      {},
    ],
    [
      'rate too wide',
      draft({
        action: 'rate_limit',
        params: { rate_count: '1', rate_window_seconds: '9999' },
      }),
      { 'params.rate_window_seconds': 'window' },
    ],
    [
      'path needs slash',
      draft({}, { value: 'admin' }),
      { 'conditions[0].value': 'slash' },
    ],
    ['glob path needs no slash', draft({}, { op: 'glob', value: '*.php' }), {}],
    [
      'too many wildcards',
      draft({}, { op: 'glob', value: '*a*a*a*a*a*a*a*a*a' }),
      { 'conditions[0].value': 'wildcards' },
    ],
    [
      'empty value',
      draft({}, { value: '' }),
      { 'conditions[0].value': 'value' },
    ],
    [
      'exists needs no value',
      draft({}, { field: 'header', op: 'exists', name: 'x-env', value: '' }),
      {},
    ],
    [
      'header needs a name',
      draft({}, { field: 'header', op: 'equals', name: '', value: 'x' }),
      { 'conditions[0].name': 'paramName' },
    ],
    [
      'header name charset',
      draft(
        {},
        { field: 'header', op: 'equals', name: 'bad name', value: 'x' },
      ),
      { 'conditions[0].name': 'paramName' },
    ],
    [
      'ip ok',
      draft({}, { field: 'ip', op: 'in_cidr', value: '10.0.0.0/8' }),
      {},
    ],
    [
      'ip v6 ok',
      draft({}, { field: 'ip', op: 'in_cidr', value: '2001:db8::/32' }),
      {},
    ],
    [
      'ip bad',
      draft({}, { field: 'ip', op: 'in_cidr', value: '999.1.1.1' }),
      { 'conditions[0].value': 'ip' },
    ],
    [
      'method bad',
      draft({}, { field: 'method', op: 'equals', value: 'FETCH' }),
      { 'conditions[0].value': 'method' },
    ],
    [
      'method lower ok',
      draft({}, { field: 'method', op: 'equals', value: 'get' }),
      {},
    ],
    [
      'country bad',
      draft({}, { field: 'country', op: 'equals', value: 'DEU' }),
      { 'conditions[0].value': 'country' },
    ],
  ]
  it.each(cases)('%s', (_n, d, want) => {
    expect(validateDraft(d, m)).toEqual(want)
  })
})

describe('isIpOrCidr', () => {
  it.each([
    ['1.2.3.4', true],
    ['1.2.3.4/32', true],
    ['1.2.3.4/33', false],
    ['256.1.1.1', false],
    ['::1', true],
    ['2001:db8::/129', false],
    ['abc', false],
  ])('%s', (v, want) => {
    expect(isIpOrCidr(v)).toBe(want)
  })
})

describe('draft and rule conversion', () => {
  it('round trips a rate limit rule and trims text', () => {
    const rule: FwRule = {
      id: 'r1',
      position: 1,
      name: 'API',
      note: 'n',
      scope: 'a.example.com',
      enabled: true,
      action: 'rate_limit',
      rate_count: 100,
      rate_window_seconds: 60,
      conditions: [
        { field: 'header', op: 'equals', name: 'x-env', value: 'prod' },
      ],
      hits: { total: 0, sparkline: [] },
    }
    const d = draftFromRule(rule)
    expect(d.params).toEqual({ rate_count: '100', rate_window_seconds: '60' })
    const back = ruleFromDraft({ ...d, name: '  API  ' }, 'a.example.com', true)
    expect(back).toEqual({
      name: 'API',
      note: 'n',
      scope: 'a.example.com',
      enabled: true,
      action: 'rate_limit',
      rate_count: 100,
      rate_window_seconds: 60,
      conditions: [
        { field: 'header', op: 'equals', name: 'x-env', value: 'prod' },
      ],
    })
  })

  it('omits value for exists and name for plain fields', () => {
    const out = ruleFromDraft(
      {
        name: 'n',
        note: '',
        action: 'log',
        params: {},
        conditions: [
          { field: 'query', op: 'exists', name: 'debug', value: 'ignored' },
          { field: 'path', op: 'equals', name: 'stale', value: '/x' },
        ],
      },
      '',
      false,
    )
    expect(out.conditions).toEqual<FwCondition[]>([
      { field: 'query', op: 'exists', name: 'debug' },
      { field: 'path', op: 'equals', value: '/x' },
    ])
    expect(out.scope).toBeUndefined()
    expect(out.enabled).toBe(false)
  })

  it('shows exactly the stored JSON', () => {
    const d = draftForValue('ip', '203.0.113.9')
    d.name = 'Block'
    d.action = 'deny'
    expect(JSON.parse(ruleJson(d, '', true))).toEqual({
      name: 'Block',
      enabled: true,
      action: 'deny',
      conditions: [{ field: 'ip', op: 'in_cidr', value: '203.0.113.9' }],
    })
  })
})

describe('server errors and helpers', () => {
  it('maps rate fields onto builder params and keeps the first message per key', () => {
    expect(
      mapServerErrors([
        { field: 'rate_count', message: 'a' },
        { field: 'name', message: 'b' },
        { field: 'name', message: 'c' },
      ]),
    ).toEqual({ 'params.rate_count': 'a', name: 'b' })
  })

  it('describes conditions compactly', () => {
    expect(
      conditionText({
        field: 'header',
        op: 'starts_with',
        name: 'x',
        value: 'v',
      }),
    ).toBe('header:x starts with v')
    expect(conditionText({ field: 'query', op: 'exists', name: 'q' })).toBe(
      'query:q exists',
    )
  })

  it('seeds a draft from a traffic row', () => {
    expect(draftForValue('path', '/a/:id').conditions[0]).toMatchObject({
      field: 'path',
      op: 'starts_with',
      value: '/a/:id',
    })
    expect(draftForValue('user_agent', 'curl').conditions[0]?.op).toBe(
      'contains',
    )
  })

  it('recognises IP list rules', () => {
    const base = {
      id: 'x',
      position: 1,
      name: 'n',
      enabled: true,
      hits: { total: 0, sparkline: [] },
    }
    expect(
      isIpListRule({
        ...base,
        action: 'deny',
        conditions: [{ field: 'ip', op: 'in_cidr', value: '1.1.1.1' }],
      }),
    ).toBe('deny')
    expect(
      isIpListRule({
        ...base,
        action: 'allow',
        conditions: [{ field: 'ip', op: 'in_cidr', value: '1.1.1.1' }],
      }),
    ).toBe('allow')
    expect(
      isIpListRule({
        ...base,
        action: 'log',
        conditions: [{ field: 'ip', op: 'in_cidr', value: '1.1.1.1' }],
      }),
    ).toBeNull()
    expect(
      isIpListRule({
        ...base,
        action: 'deny',
        conditions: [{ field: 'path', op: 'equals', value: '/' }],
      }),
    ).toBeNull()
  })

  it('moves a custom rule past its custom neighbour inside the full order', () => {
    const mk = (id: string): FwRule => ({
      id,
      position: 1,
      name: id,
      enabled: true,
      action: 'deny',
      conditions: [],
      hits: { total: 0, sparkline: [] },
    })
    const all = [mk('ip1'), mk('a'), mk('ip2'), mk('b')]
    const custom = [all[1]!, all[3]!]
    expect(movedOrder(all, custom, 'b', -1)).toEqual(['ip1', 'b', 'ip2', 'a'])
    expect(movedOrder(all, custom, 'a', 1)).toEqual(['ip1', 'b', 'ip2', 'a'])
    expect(movedOrder(all, custom, 'a', -1)).toBeNull()
    expect(movedOrder(all, custom, 'b', 1)).toBeNull()
  })
})
