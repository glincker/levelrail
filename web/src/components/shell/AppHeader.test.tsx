import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { AttentionItem } from '../../lib/attention'
import commonEn from '../../locales/en/common.json'
import { AppHeader } from './AppHeader'
import { SkipLink } from './SkipLink'
import { StatusChip } from './StatusChip'
import { resetSnoozeStoreForTests } from './snoozeStore'

const attention = vi.hoisted(() => ({ items: [] as AttentionItem[] }))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouterState: () => undefined,
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('../../queries/attention', () => ({
  useAttentionItems: () => ({ items: attention.items, isLoading: false }),
}))
vi.mock('../../queries/silences', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../queries/silences')>()),
  useSilences: () => ({ data: [] }),
  useCreateSilence: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteSilence: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('../../lib/connection', () => ({
  isInsecureRemoteConnection: () => false,
}))
vi.mock('../ui/sidebar', () => ({
  SidebarTrigger: (props: { 'aria-label'?: string }) => (
    <button type="button" aria-label={props['aria-label']} />
  ),
}))
vi.mock('../teams/TeamSwitcher', () => ({ TeamSwitcher: () => null }))
vi.mock('../environments/ScopePicker', () => ({
  ScopePicker: () => null,
}))
vi.mock('../HelpMenu', () => ({ HelpMenu: () => null }))
vi.mock('../ThemeToggle', () => ({ ThemeToggle: () => null }))
vi.mock('./NotificationCenter', () => ({ NotificationCenter: () => null }))
vi.mock('../cli/CliStatusBadge', () => ({ CliStatusBadge: () => null }))
vi.mock('../cli/CliModal', () => ({ CliModal: () => null }))
vi.mock('./InboxBadge', () => ({ InboxBadge: () => null }))
vi.mock('./WhatsNewPanel', () => ({ WhatsNewPanel: () => null }))
vi.mock('../CleanUpDockerDialog', () => ({ CleanUpDockerDialog: () => null }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

const item = (
  id: string,
  severity: AttentionItem['severity'],
): AttentionItem => ({
  id,
  severity,
  title: id,
  detail: '',
  target: { kind: 'system' },
})

function renderShell(node: React.ReactNode) {
  return render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>)
}

beforeEach(() => {
  resetSnoozeStoreForTests()
  attention.items = [
    item('a', 'critical'),
    item('b', 'warning'),
    item('c', 'warning'),
    item('u', 'info'),
  ]
})

describe('AppHeader', () => {
  it('exposes the search trigger and attention chip with translated labels', () => {
    const onSearch = vi.fn()
    renderShell(<AppHeader onSearch={onSearch} />)
    expect(screen.getByRole('banner')).toHaveAccessibleName('Top bar')
    fireEvent.click(
      screen.getByRole('button', { name: 'Search or run a command' }),
    )
    expect(onSearch).toHaveBeenCalledOnce()
    expect(
      screen.getByRole('button', { name: 'Platform status: 3 issues' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Toggle sidebar' }),
    ).toBeInTheDocument()
  })

  it('does not print the quick navigation hint', () => {
    renderShell(<AppHeader onSearch={vi.fn()} />)
    expect(screen.queryByText(/for quick navigation/)).toBeNull()
  })
})

describe('StatusChip', () => {
  it('shows all good when nothing needs attention', () => {
    attention.items = [item('u', 'info')]
    renderShell(<StatusChip />)
    expect(
      screen.getByRole('button', { name: 'Platform status: All good' }),
    ).toBeInTheDocument()
  })
})

describe('SkipLink', () => {
  it('moves focus to the main region', () => {
    renderShell(
      <>
        <SkipLink />
        <main id="main-content" tabIndex={-1} />
      </>,
    )
    Element.prototype.scrollIntoView = vi.fn()
    fireEvent.click(screen.getByRole('link', { name: 'Skip to content' }))
    expect(document.getElementById('main-content')).toHaveFocus()
  })
})
