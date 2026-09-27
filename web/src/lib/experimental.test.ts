import { describe, expect, it } from 'vitest'
import {
  visibleAppSections,
  visibleGlobalFooter,
  visibleGlobalGroups,
} from '../components/shell/navModel'
import { visibleSettingsSections } from './settingsNav'
import { isFeatureVisible } from './experimental'

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
  const tos = () =>
    [
      ...visibleGlobalGroups([]).flatMap((g) => g.items),
      ...visibleGlobalFooter([]),
    ].map((i) => i.to)

  it.each(gatedGlobal)('hides %s by default and shows it when on', (to, f) => {
    expect(tos()).not.toContain(to)
    const on = [
      ...visibleGlobalGroups([f]).flatMap((g) => g.items),
      ...visibleGlobalFooter([f]),
    ].map((i) => i.to)
    expect(on).toContain(to)
  })

  it('hides the AI group entirely when no AI feature is on', () => {
    expect(visibleGlobalGroups([]).map((g) => g.id)).not.toContain('ai')
  })

  it('hides the per-app load balancer entry by default', () => {
    const slugs = (e: string[]) =>
      visibleAppSections(e).flatMap((s) => s.items.map((i) => i.slug))
    expect(slugs([])).not.toContain('loadbalancer')
    expect(slugs(['load-balancer'])).toContain('loadbalancer')
  })

  it('gates the settings pages', () => {
    const tos = (e: string[]) =>
      visibleSettingsSections(e).flatMap((s) => s.items.map((i) => i.to))
    const cases: [string, string][] = [
      ['/settings/cloudflare-tunnel', 'cloudflare-tunnel'],
      ['/settings/ai-assistant', 'ai-chat'],
      ['/settings/infrastructure', 'iac'],
    ]
    for (const [to, f] of cases) {
      expect(tos([])).not.toContain(to)
      expect(tos([f])).toContain(to)
    }
  })
})
