import { describe, expect, it } from 'vitest'
import type { AiAssistantConfig } from '../../queries/aiAssistantSettings'
import type { AiProvider } from '../../queries/aiProviders'
import {
  addFallback,
  checkDraft,
  deriveHeader,
  groupProviders,
  moveItem,
  pillState,
  tokensPerDay,
} from './aiProviderState'

function prov(over: Partial<AiProvider> = {}): AiProvider {
  return {
    id: 'openai',
    label: 'OpenAI',
    kind: 'openai_compat',
    default_base_url: '',
    base_url_editable: false,
    key_url: '',
    brand: '',
    has_key: true,
    status: 'ok',
    default_model: 'gpt-x',
    ...over,
  }
}

const cfg = (over: Partial<AiAssistantConfig> = {}): AiAssistantConfig => ({
  configured: true,
  fallback: [],
  limits: { max_steps: 12 },
  ...over,
})

describe('deriveHeader', () => {
  it.each([
    ['no providers', [], undefined, 'not_configured'],
    [
      'none connected',
      [prov({ has_key: false })],
      cfg({ configured: false }),
      'not_configured',
    ],
    [
      'connected with active',
      [prov()],
      cfg({ active: { provider: 'openai', model: 'm' } }),
      'ready',
    ],
    ['connected, no active', [prov()], cfg(), 'attention'],
    [
      'invalid key on active',
      [prov({ status: 'invalid_key' })],
      cfg({ active: { provider: 'openai', model: 'm' } }),
      'attention',
    ],
    [
      'unreachable on active',
      [prov({ status: 'unreachable' })],
      cfg({ active: { provider: 'openai', model: 'm' } }),
      'attention',
    ],
    [
      'out of credits on active',
      [prov({ status: 'quota_exhausted' })],
      cfg({ active: { provider: 'openai', model: 'm' } }),
      'attention',
    ],
    [
      'active provider gone',
      [prov({ id: 'other' })],
      cfg({ active: { provider: 'openai', model: 'm' } }),
      'attention',
    ],
  ])('%s', (_n, providers, config, want) => {
    expect(deriveHeader(providers, config).kind).toBe(want)
  })
})

describe('pillState', () => {
  it.each([
    [{ has_key: false }, 'not_connected'],
    [{ status: 'untested' as const }, 'connected'],
    [{ status: 'invalid_key' as const }, 'invalid_key'],
    [{ status: 'unreachable' as const }, 'unreachable'],
    [{ status: 'quota_exhausted' as const }, 'quota_exhausted'],
  ])('%j', (over, want) => {
    expect(pillState(prov(over))).toBe(want)
  })
})

describe('groupProviders', () => {
  it('puts connected first and splits self-hosted', () => {
    const g = groupProviders([
      prov({ id: 'a', has_key: false }),
      prov({ id: 'ollama', base_url_editable: true, has_key: false }),
      prov({ id: 'b' }),
    ])
    expect(g.hosted.map((p) => p.id)).toEqual(['b', 'a'])
    expect(g.selfHosted.map((p) => p.id)).toEqual(['ollama'])
  })
})

describe('fallback ordering', () => {
  it.each([
    [[1, 2, 3], 0, 1, [2, 1, 3]],
    [[1, 2, 3], 2, 0, [3, 1, 2]],
    [[1, 2, 3], 0, 0, [1, 2, 3]],
    [[1, 2, 3], 1, 9, [1, 3, 2]],
    [[1, 2, 3], 7, 0, [1, 2, 3]],
  ])('moveItem %j %i->%i', (list, from, to, want) => {
    expect(moveItem(list, from, to)).toEqual(want)
  })

  it('addFallback skips the active model and duplicates', () => {
    const active = { provider: 'a', model: 'm' }
    const f = [{ provider: 'b', model: 'n' }]
    expect(addFallback(f, active, active)).toEqual(f)
    expect(addFallback(f, active, f[0]!)).toEqual(f)
    expect(addFallback(f, active, { provider: 'c', model: 'o' })).toHaveLength(
      2,
    )
  })
})

describe('checkDraft', () => {
  const empty = { apiKey: '', baseUrl: '', model: 'gpt-x' }
  it.each([
    ['new provider without key', prov({ has_key: false }), empty, false],
    [
      'new provider with key',
      prov({ has_key: false }),
      { ...empty, apiKey: 'sk' },
      true,
    ],
    ['saved, nothing changed', prov(), empty, false],
    ['saved, model changed', prov(), { ...empty, model: 'other' }, true],
    ['saved, new key', prov(), { ...empty, apiKey: 'sk' }, true],
    [
      'self-hosted without key',
      prov({ has_key: false, base_url_editable: true }),
      { ...empty, baseUrl: 'http://10.0.0.5:11434' },
      true,
    ],
    [
      'bad base url',
      prov({ base_url_editable: true }),
      { ...empty, baseUrl: 'nope' },
      false,
    ],
  ])('%s', (_n, p, d, valid) => {
    expect(checkDraft(p, d).valid).toBe(valid)
  })
})

describe('tokensPerDay', () => {
  it('fills a zero for days without usage', () => {
    const end = new Date('2026-10-16T12:00:00Z')
    const pts = tokensPerDay(
      [
        {
          date: '2026-10-16',
          provider: 'a',
          model: 'm',
          prompt_tokens: 5,
          completion_tokens: 7,
          requests: 1,
          errors: 0,
        },
        {
          date: '2026-10-16',
          provider: 'b',
          model: 'm',
          prompt_tokens: 1,
          completion_tokens: 0,
          requests: 1,
          errors: 0,
        },
      ],
      3,
      end,
    )
    expect(pts.map((p) => p.tokens)).toEqual([0, 0, 13])
  })
})
