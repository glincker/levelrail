import { describe, expect, it } from 'vitest'
import { Route as Overview } from '../../routes/settings/platform/index'
import { Route as Https } from '../../routes/settings/platform/https'
import { Route as Ingress } from '../../routes/settings/platform/ingress'
import { Route as Addresses } from '../../routes/settings/platform/addresses'
import { Route as Dns } from '../../routes/settings/platform/dns-providers'
import { ROUTE_ENTRIES, routeEntryItem } from '../commandPaletteData'
import { settingsNavSections } from '../../lib/settingsNav'

describe('platform routes', () => {
  it.each([
    ['overview', Overview],
    ['https', Https],
    ['ingress', Ingress],
    ['addresses', Addresses],
    ['dns-providers', Dns],
  ])('registers a component and loader for %s', (_name, route) => {
    expect(route.options.component).toBeTypeOf('function')
    expect(route.options.loader).toBeTypeOf('function')
  })
})

describe('platform nav and palette', () => {
  it('lists five pages in a Platform settings area', () => {
    const area = settingsNavSections.find((s) => s.id === 'platform')
    expect(area?.items.map((i) => i.to)).toEqual([
      '/settings/platform',
      '/settings/platform/https',
      '/settings/platform/ingress',
      '/settings/platform/addresses',
      '/settings/platform/dns-providers',
    ])
  })

  it('has palette entries for pages and individual settings', () => {
    const labels = ROUTE_ENTRIES.filter((e) =>
      e.key.startsWith('settings-platform'),
    ).map((e) => e.label)
    for (const l of [
      'Dashboard address',
      'Enable HTTPS',
      'Apps base domain',
      'Fallback domains',
      'Ingress settings',
      'Reverse proxy guide',
      'Cloudflare DNS',
      'Route 53',
      'Google Cloud DNS',
      'Platform settings',
    ]) {
      expect(labels).toContain(l)
    }
  })

  it('jumps to the anchor when a setting entry runs', () => {
    const entry = ROUTE_ENTRIES.find(
      (e) => e.key === 'settings-platform-dashboard-address',
    )
    expect(entry).toBeDefined()
    const calls: unknown[][] = []
    const item = routeEntryItem(
      entry!,
      [],
      (...args) =>
        () => {
          calls.push(args)
        },
      'locked',
    )
    item.run()
    expect(calls[0]).toEqual([
      '/settings/platform/addresses',
      undefined,
      undefined,
      'dashboard-url',
    ])
  })
})
