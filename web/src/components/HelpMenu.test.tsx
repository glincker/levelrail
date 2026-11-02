import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HelpMenu } from './HelpMenu'

let isRoot = true

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useRouterState: ({
    select,
  }: {
    select: (s: { location: { pathname: string } }) => unknown
  }) => select({ location: { pathname: '/' } }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../hooks/useBrand', () => ({ useBrand: () => ({}) }))
vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => isRoot }))

describe('HelpMenu get started entry', () => {
  beforeEach(() => {
    isRoot = true
  })

  it('links to Get started so a dismissed guide stays reachable', async () => {
    render(<HelpMenu />)
    await userEvent.setup().click(screen.getByRole('button'))
    const item = await screen.findByText('shell.help.getStarted')
    expect(item.closest('a')?.getAttribute('href')).toBe('/get-started')
  })

  it('hides it for accounts that are not root', async () => {
    isRoot = false
    render(<HelpMenu />)
    await userEvent.setup().click(screen.getByRole('button'))
    await screen.findByText('shell.help.docs')
    expect(screen.queryByText('shell.help.getStarted')).toBeNull()
  })
})
