import { describe, expect, it } from 'vitest'
import {
  visibleAppSections,
  visibleGlobalFooter,
  visibleGlobalGroups,
} from '../components/shell/navModel'
import { visibleSettingsSections } from './settingsNav'
import { isRedirect } from '@tanstack/react-router'
import { QueryClient } from '@tanstack/react-query'
import { devModeQueryOptions } from '../queries/devMode'
import {
  isFeatureLocked,
  isFeatureVisible,
  lockedSearch,
  requireExperimental,
} from './experimental'

const gatedGlobal = [
  ['/loadbalancers', 'load-balancer'],
  ['/models', 'ai-models'],
  ['/ai-assistant', 'ai-chat'],
] as const

describe('isFeatureVisible', () => {
  it.each([
    [undefined, [], true],
    ['iac', [], false],
    ['iac', ['iac'], true],
    ['iac', ['ai-chat'], false],
  ] as const)('%s with %j is %s', (feature, enabled, want) => {
    expect(isFeatureVisible(feature, enabled)).toBe(want)
  })
})

describe('experimental navigation', () => {
  const allItems = () => [
    ...visibleGlobalGroups().flatMap((g) => g.items),
    ...visibleGlobalFooter(),
  ]

  it.each(gatedGlobal)('keeps %s listed and locked until %s is on', (to, f) => {
    const item = allItems().find((i) => i.to === to)
    expect(item?.feature).toBe(f)
    expect(isFeatureLocked(item?.feature, [])).toBe(true)
    expect(isFeatureLocked(item?.feature, [f])).toBe(false)
  })

  it('keeps the AI group so its locked entries stay discoverable', () => {
    expect(visibleGlobalGroups().map((g) => g.id)).toContain('ai')
  })

  it('lists the per-app load balancer entry for the renderer to lock', () => {
    const lb = visibleAppSections()
      .flatMap((s) => s.items)
      .find((i) => i.slug === 'loadbalancer')
    expect(lb?.feature).toBe('load-balancer')
  })

  it('lists gated settings pages with their feature', () => {
    const items = visibleSettingsSections().flatMap((s) => s.items)
    const cases: [string, string][] = [
      ['/settings/cloudflare-tunnel', 'cloudflare-tunnel'],
      ['/settings/ai-assistant', 'ai-chat'],
      ['/settings/infrastructure', 'iac'],
    ]
    for (const [to, f] of cases) {
      expect(items.find((i) => i.to === to)?.feature).toBe(f)
    }
  })
})

describe('locked destination targets', () => {
  it.each([
    [
      'load-balancer',
      '/loadbalancers',
      { feature: 'load-balancer', from: '/loadbalancers' },
    ],
    ['iac', undefined, { feature: 'iac' }],
    ['iac', '//evil.example', { feature: 'iac' }],
    ['iac', 'https://evil.example', { feature: 'iac' }],
    [
      'ai-chat',
      '/apps/web?tab=1',
      { feature: 'ai-chat', from: '/apps/web?tab=1' },
    ],
  ] as const)('%s from %s', (feature, from, want) => {
    expect(lockedSearch(feature, from)).toEqual(want)
  })
})

describe('requireExperimental', () => {
  const clientWith = (experimental: string[]) => {
    const client = new QueryClient()
    client.setQueryData(devModeQueryOptions().queryKey, {
      enabled: false,
      experimental,
    })
    return client
  }

  it('passes when the feature is on', async () => {
    await expect(
      requireExperimental(
        clientWith(['iac']),
        'iac',
        '/settings/infrastructure',
      ),
    ).resolves.toBeUndefined()
  })

  it('redirects to Beta features carrying the feature and return path', async () => {
    const err: unknown = await requireExperimental(
      clientWith([]),
      'load-balancer',
      '/loadbalancers',
    ).catch((e: unknown) => e)
    expect(isRedirect(err)).toBe(true)
    expect(err).toMatchObject({
      options: {
        to: '/settings/beta-features',
        search: { feature: 'load-balancer', from: '/loadbalancers' },
      },
    })
  })
})
