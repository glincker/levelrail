import { describe, expect, it } from 'vitest'
import { Route as Layout } from '../../routes/settings/platform/route'
import { Route as Overview } from '../../routes/settings/platform/index'
import { Route as Https } from '../../routes/settings/platform/https'
import { Route as Ingress } from '../../routes/settings/platform/ingress'
import { Route as Addresses } from '../../routes/settings/platform/addresses'
import { Route as Dns } from '../../routes/settings/platform/dns-providers'
import { ROUTE_ENTRIES, routeEntryItem } from '../commandPaletteData'
import { settingsNavSections } from '../../lib/settingsNav'
import { PLATFORM_TAB_PATHS } from '../../lib/platformSettingsIndex'

describe('platform routes', () => {
  it('prefetches shared data once in the layout route', () => {
    expect(Layout.options.component).toBeTypeOf('function')
    expect(Layout.options.loader).toBeTypeOf('function')
  })

  it.each([
    ['overview', Overview],
    ['https', Https],
    ['ingress', Ingress],
    ['addresses', Addresses],
    ['dns-providers', Dns],
  ])('registers a component for %s', (_name, route) => {
    expect(route.options.component).toBeTypeOf('function')
  })
})

describe('platform nav and palette', () => {
  it('has one Platform settings sidebar item and five tabs', () => {
    const items = settingsNavSections
      .flatMap((s) => s.items)
      .filter((i) => i.to.startsWith('/settings/platform'))
    expect(items.map((i) => i.to)).toEqual(['/settings/platform'])
    expect(PLATFORM_TAB_PATHS).toHaveLength(5)
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
