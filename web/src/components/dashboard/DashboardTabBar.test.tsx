import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useDashboardTabs } from '../../hooks/useDashboardTabs'
import dashboardEn from '../../locales/en/dashboard.json'
import { DashboardTabBar } from './DashboardTabBar'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['dashboard'],
  defaultNS: 'dashboard',
  resources: { en: { dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})

function Harness({ ids }: { ids: string[] }) {
  const tabs = useDashboardTabs(ids)
  const labelOf = (id: string) =>
    id === 'start' ? 'Start' : id === 'metrics' ? 'Metrics' : id.slice(2)
  return (
    <>
      <DashboardTabBar
        shown={tabs.shown}
        hidden={tabs.hidden}
        selected={tabs.selected}
        labelOf={labelOf}
        onSelect={tabs.select}
        onAdd={vi.fn()}
        onHide={tabs.hide}
        onShow={tabs.show}
        onMove={tabs.move}
      />
      <p data-testid="selected">{tabs.selected}</p>
    </>
  )
}

function renderBar(ids: string[]) {
  return render(
    <I18nextProvider i18n={i18n}>
      <Harness ids={ids} />
    </I18nextProvider>,
  )
}

describe('DashboardTabBar', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('selects a tab and restores it on remount', async () => {
    const user = userEvent.setup()
    const { unmount } = renderBar(['a'])
    expect(screen.getByRole('tab', { name: 'Start' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await user.click(screen.getByRole('tab', { name: 'a' }))
    expect(window.localStorage.getItem('dashboard.tab.v1')).toBe('d:a')
    unmount()
    renderBar(['a'])
    expect(screen.getByTestId('selected')).toHaveTextContent('d:a')
  })

  it('moves selection with arrow keys', async () => {
    const user = userEvent.setup()
    renderBar([])
    screen.getByRole('tab', { name: 'Start' }).focus()
    await user.keyboard('{ArrowRight}')
    expect(screen.getByRole('tab', { name: 'Metrics' })).toHaveFocus()
    expect(screen.getByTestId('selected')).toHaveTextContent('metrics')
  })

  it('shows four tabs and the rest in the overflow menu', async () => {
    const user = userEvent.setup()
    renderBar(['a', 'b', 'c'])
    expect(screen.getAllByRole('tab')).toHaveLength(4)
    await user.click(screen.getByRole('button', { name: 'More views' }))
    await user.click(await screen.findByRole('menuitem', { name: 'c' }))
    expect(screen.getByRole('tab', { name: 'c' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(screen.getAllByRole('tab')).toHaveLength(4)
  })

  it('renders without storage', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    renderBar(['a'])
    expect(screen.getByTestId('selected')).toHaveTextContent('start')
  })

  it('hides a tab with Delete and persists it', async () => {
    const user = userEvent.setup()
    renderBar([])
    await user.click(screen.getByRole('tab', { name: 'Metrics' }))
    await user.keyboard('{Delete}')
    expect(screen.queryByRole('tab', { name: 'Metrics' })).toBeNull()
    expect(window.localStorage.getItem('dashboard.tabs.v1')).toContain(
      'metrics',
    )
  })
})
