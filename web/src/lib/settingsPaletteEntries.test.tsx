import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import settingsEn from '../locales/en/settings.json'
import paletteSettingsEn from '../locales/en/paletteSettings.json'
import { rankItems } from './paletteSearch'
import { settingsNavSections } from './settingsNav'
import { useSettingsPaletteEntries } from './settingsPaletteEntries'
import { mergeSettingsEntries } from '../components/commandPaletteData'

let prefix = ''

function lookup(table: unknown, key: string): string {
  let cur: unknown = table
  for (const part of key.split('.')) {
    cur =
      cur && typeof cur === 'object'
        ? (cur as Record<string, unknown>)[part]
        : undefined
  }
  return typeof cur === 'string' ? cur : key
}

vi.mock('react-i18next', () => ({
  useTranslation: (ns?: string) => ({
    t: (key: string) =>
      `${prefix}${lookup(ns === 'paletteSettings' ? paletteSettingsEn : settingsEn, key)}`,
  }),
}))
vi.mock('../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => [],
}))

// Top level settings route files that are deliberately not nav pages.
const NOT_NAV_PAGES: Record<string, string> = {
  janitor: 'redirects to the Cleanup page',
}

const allItems = settingsNavSections.flatMap((s) =>
  s.items.map((i) => ({ section: s.id, item: i })),
)

beforeEach(() => {
  prefix = ''
})

describe('settings palette entries', () => {
  it('lists every settingsNav item', () => {
    const { result } = renderHook(() => useSettingsPaletteEntries())
    const keys = result.current.map((e) => e.key)
    expect(new Set(keys).size).toBe(keys.length)
    expect(result.current.map((e) => e.to).sort()).toEqual(
      allItems.map((a) => a.item.to).sort(),
    )
    for (const { item } of allItems) {
      expect(keys).toContain(`settings-${item.id}`)
    }
  })

  it('translates labels and group names through t', () => {
    prefix = 'T:'
    const { result } = renderHook(() => useSettingsPaletteEntries())
    for (const e of result.current) {
      expect(e.label.startsWith('T:')).toBe(true)
      expect(e.subtitle?.startsWith('T:')).toBe(true)
    }
  })

  it('groups each page under its settingsNav area', () => {
    const { result } = renderHook(() => useSettingsPaletteEntries())
    for (const { section, item } of allItems) {
      const e = result.current.find((r) => r.key === `settings-${item.id}`)
      expect(e?.subtitle).toBe(settingsEn.nav.areas[section].title)
      expect(e?.group).toBe('Settings')
    }
  })

  it('finds a page by its name and by its area', () => {
    const { result } = renderHook(() => useSettingsPaletteEntries())
    expect(
      rankItems(result.current, 'certificates', (e) => e.keywords).map(
        (e) => e.key,
      ),
    ).toContain('settings-certificates')
    expect(
      rankItems(result.current, 'delivery log', (e) => e.keywords).map(
        (e) => e.key,
      ),
    ).toContain('settings-email-log')
  })

  it.each([
    ['appearance', 'settings-appearance'],
    ['api tokens', 'settings-tokens'],
    ['certificate budget', 'settings-certificates'],
  ])('finds %s', (query, key) => {
    const { result } = renderHook(() => useSettingsPaletteEntries())
    expect(
      rankItems(result.current, query, (e) => e.keywords).map((e) => e.key),
    ).toContain(key)
  })

  it('does not list a nav destination twice', () => {
    const { result } = renderHook(() => useSettingsPaletteEntries())
    const tos = mergeSettingsEntries(result.current)
      .filter((e) => e.to === '/domains')
      .map((e) => e.key)
    expect(tos).toEqual(['nav-domains'])
  })
})

describe('settings route coverage', () => {
  it('has every top level settings route in settingsNav or the allowlist', () => {
    const dir = join(__dirname, '..', 'routes', 'settings')
    const files = readdirSync(dir)
      .filter((f) => f.endsWith('.tsx') && !f.endsWith('.test.tsx'))
      .map((f) => f.replace(/\.tsx$/, ''))
      .filter((f) => f !== 'index')
    const navPaths = new Set(allItems.map((a) => a.item.to))
    const missing = files.filter(
      (f) => !navPaths.has(`/settings/${f}`) && !(f in NOT_NAV_PAGES),
    )
    expect(missing).toEqual([])
  })
})
