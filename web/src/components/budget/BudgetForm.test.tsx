import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { BudgetForm } from './BudgetForm'
import budgetEn from '../../locales/en/budget.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['budget'],
  defaultNS: 'budget',
  resources: { en: { budget: budgetEn } },
  interpolation: { escapeValue: false },
})

function setup() {
  const onSave = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <BudgetForm
        cap={null}
        pending={false}
        serverError={null}
        onSave={onSave}
        onCancel={vi.fn()}
      />
    </I18nextProvider>,
  )
  return { onSave, user: userEvent.setup() }
}

describe('BudgetForm', () => {
  it('asks for a limit before saving', async () => {
    const { onSave, user } = setup()
    await user.click(screen.getByRole('button', { name: 'Save budget' }))
    expect(screen.getByRole('alert').textContent).toContain('Set an egress')
    expect(onSave).not.toHaveBeenCalled()
  })

  it('rejects out of range alert levels', async () => {
    const { onSave, user } = setup()
    await user.type(screen.getByLabelText('Monthly egress limit (GiB)'), '10')
    await user.clear(screen.getByLabelText('Alert levels (percent)'))
    await user.type(screen.getByLabelText('Alert levels (percent)'), '50, 150')
    await user.click(screen.getByRole('button', { name: 'Save budget' }))
    expect(screen.getByRole('alert').textContent).toContain('whole percents')
    expect(onSave).not.toHaveBeenCalled()
  })

  it('sends bytes, levels and the chosen action', async () => {
    const { onSave, user } = setup()
    await user.type(screen.getByLabelText('Monthly egress limit (GiB)'), '2')
    await user.click(screen.getByRole('radio', { name: /Throttle/ }))
    await user.click(screen.getByRole('button', { name: 'Save budget' }))
    expect(onSave).toHaveBeenCalledWith({
      monthly_egress_bytes: 2 * 1024 ** 3,
      monthly_cost_ceiling: 0,
      thresholds: [50, 80, 100],
      action: 'throttle',
      throttle_rps: 5,
      guard_mode: 'alert',
    })
  })

  it('sends the deploy guard mode when chosen', async () => {
    const { onSave, user } = setup()
    await user.type(screen.getByLabelText('Monthly cost ceiling'), '40')
    await user.click(screen.getByRole('radio', { name: /Block new deploys/ }))
    await user.click(screen.getByRole('button', { name: 'Save budget' }))
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        monthly_cost_ceiling: 40,
        guard_mode: 'block_deploys',
      }),
    )
  })
})
