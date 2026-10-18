import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import dashboardsEn from '../../locales/en/dashboards.json'
import type { DashboardWidget } from '../../types/dashboards'
import { DashboardGrid } from './DashboardGrid'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['dashboards'],
  defaultNS: 'dashboards',
  resources: { en: { dashboards: dashboardsEn } },
  interpolation: { escapeValue: false },
})

const widgets: DashboardWidget[] = [
  { id: 'a', type: 'resource_counts', size: 'M', title: 'Counts' },
  { id: 'b', type: 'log_tail', size: 'L', title: 'Logs' },
]

function setup(editing: boolean) {
  const onChange = vi.fn()
  render(
    <I18nextProvider i18n={i18n}>
      <DashboardGrid
        widgets={widgets}
        editing={editing}
        onChange={onChange}
        renderBody={(w) => <p>body of {w.id}</p>}
      />
    </I18nextProvider>,
  )
  return onChange
}

describe('DashboardGrid', () => {
  it('renders every widget without edit controls in view mode', () => {
    setup(false)
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('body of b')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('moves, resizes and removes with plain buttons in edit mode', async () => {
    const user = userEvent.setup()
    const onChange = setup(true)

    await user.click(screen.getByRole('button', { name: 'Move Counts later' }))
    expect(onChange).toHaveBeenLastCalledWith([widgets[1], widgets[0]])

    await user.click(screen.getByRole('button', { name: 'Make Logs size S' }))
    expect(onChange).toHaveBeenLastCalledWith([
      widgets[0],
      { ...widgets[1], size: 'S' },
    ])

    await user.click(screen.getByRole('button', { name: 'Remove Counts' }))
    expect(onChange).toHaveBeenLastCalledWith([widgets[1]])
  })

  it('disables moving past either end', () => {
    setup(true)
    expect(
      screen.getByRole('button', { name: 'Move Counts earlier' }),
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Move Logs later' }),
    ).toBeDisabled()
  })
})
