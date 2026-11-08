import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SidebarCustomizeDialog } from './SidebarCustomizeDialog'
import { CommonI18n } from '@/test/commonI18n'
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
      <CommonI18n>
        <SidebarCustomizeDialog open onOpenChange={() => undefined} />
      </CommonI18n>,
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
      <CommonI18n>
        <SidebarCustomizeDialog open onOpenChange={() => undefined} />
      </CommonI18n>,
    )
    await user.click(screen.getByRole('button', { name: 'Move Alerts up' }))
    expect(screen.getByRole('status')).toHaveTextContent(/moved to position/)
    expect(Object.keys(stored().itemOrder).length).toBe(1)
  })
})
