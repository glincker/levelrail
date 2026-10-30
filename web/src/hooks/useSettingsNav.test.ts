import { describe, expect, it } from 'vitest'
import {
  filterSettingsNav,
  type LabelledSettingsSection,
} from './useSettingsNav'
import { settingsNavSections } from '../lib/settingsNav'
import settingsEn from '../locales/en/settings.json'

const sections: LabelledSettingsSection[] = [
  {
    id: 'security',
    title: 'Security and access',
    description: '',
    items: [
      {
        id: 'users',
        to: '/settings/users',
        icon: () => null,
        locked: false,
        title: 'Users',
        description: 'People who can sign in.',
      },
    ],
  },
  {
    id: 'domains',
    title: 'Domains and TLS',
    description: '',
    items: [
      {
        id: 'certificates',
        to: '/settings/certificates',
        icon: () => null,
        locked: false,
        title: 'TLS certificates',
        description: 'Expiry and renewal.',
      },
    ],
  },
]

describe('filterSettingsNav', () => {
  it('returns everything for a blank query', () => {
    expect(filterSettingsNav(sections, '  ')).toHaveLength(2)
  })

  it('matches item titles and descriptions', () => {
    expect(filterSettingsNav(sections, 'renewal').map((s) => s.id)).toEqual([
      'domains',
    ])
    expect(filterSettingsNav(sections, 'USERS')[0]?.items).toHaveLength(1)
  })

  it('keeps a whole area when its name matches', () => {
    expect(filterSettingsNav(sections, 'security')[0]?.id).toBe('security')
  })
})

describe('settings nav catalogue', () => {
  it('keeps Platform as one item and no longer has its own area', () => {
    expect('platform' in settingsEn.nav.areas).toBe(false)
    const platformItems = settingsNavSections
      .flatMap((a) => a.items)
      .filter((i) => i.to.startsWith('/settings/platform'))
    expect(platformItems).toHaveLength(1)
  })

  it('has a label for every area and item, with unique paths', () => {
    const paths = new Set<string>()
    for (const area of settingsNavSections) {
      expect(settingsEn.nav.areas[area.id].title).toBeTruthy()
      for (const item of area.items) {
        expect(settingsEn.nav.items[item.id].title).toBeTruthy()
        expect(paths.has(item.to)).toBe(false)
        paths.add(item.to)
      }
    }
  })
})
