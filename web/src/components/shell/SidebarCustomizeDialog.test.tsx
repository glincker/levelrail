import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SidebarCustomizeDialog } from './SidebarCustomizeDialog'
import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '@/locales/en/common.json'
import customizeEn from '@/locales/en/sidebarCustomize.json'
import { NAV_PREFS_STORAGE_KEY, parseNavPrefs } from '@/lib/navPrefs'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useRouterState: () => '/apps' }
})
vi.mock('@/lib/routeAvailability', () => ({
  useRouteAvailable: () => () => true,
}))

function stored() {
  return parseNavPrefs(window.localStorage.getItem(NAV_PREFS_STORAGE_KEY))
}

describe('SidebarCustomizeDialog', () => {
  beforeEach(() => window.localStorage.clear())

  it('hides a page, blocks the current one and resets', async () => {
    const user = userEvent.setup()
    render(
      <Wrap>
        <SidebarCustomizeDialog open onOpenChange={() => undefined} />
      </Wrap>,
    )
    expect(
      screen.getByRole('dialog', { name: 'Customize sidebar' }),
    ).toBeInTheDocument()

    const current = screen.getByRole('switch', { name: 'Show Apps' })
    expect(current).toBeChecked()
    expect(current).toHaveAttribute('aria-disabled', 'true')

    const status = screen.getByRole('switch', { name: 'Show Alerts' })
    await user.click(status)
    expect(stored().hiddenItems).toContain('alerts')

    await user.click(screen.getByRole('button', { name: 'Reset to default' }))
    expect(window.localStorage.getItem(NAV_PREFS_STORAGE_KEY)).toBeNull()
  })

  it('moves a page with the keyboard buttons and announces it', async () => {
    const user = userEvent.setup()
    render(
      <Wrap>
        <SidebarCustomizeDialog open onOpenChange={() => undefined} />
      </Wrap>,
    )
    await user.click(screen.getByRole('button', { name: 'Move Alerts up' }))
    expect(screen.getByRole('status')).toHaveTextContent(/moved to position/)
    expect(Object.keys(stored().itemOrder).length).toBe(1)
  })
})

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common', 'sidebarCustomize'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, sidebarCustomize: customizeEn } },
  interpolation: { escapeValue: false },
})

function Wrap({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
