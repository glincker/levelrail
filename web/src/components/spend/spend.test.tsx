import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { PausePreviewDialog } from './PausePreviewDialog'
import { SpendMeter } from './SpendMeter'
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

function wrap(node: React.ReactNode) {
  return render(<I18nextProvider i18n={testI18n}>{node}</I18nextProvider>)
}

describe('SpendMeter', () => {
  it('flags a projection past the limit', () => {
    wrap(
      <SpendMeter
        label="Cost"
        detail="10 of 40"
        pct={25}
        projectedPct={130}
        projected="52.00"
      />,
    )
    expect(screen.getByText(/Projected 52.00 by month end/)).toBeTruthy()
    expect(screen.getByText(/over the limit/)).toBeTruthy()
  })

  it('stays quiet without a projection', () => {
    wrap(<SpendMeter label="Cost" detail="10 of 40" pct={25} />)
    expect(screen.queryByText(/Projected/)).toBeNull()
  })
})

describe('PausePreviewDialog', () => {
  it('lists apps, replicas and the saving, and says nothing was changed', () => {
    wrap(
      <PausePreviewDialog
        open
        onOpenChange={vi.fn()}
        pending={false}
        error={null}
        preview={{
          apps: [
            {
              name: 'web',
              replicas: 2,
              daily_run_rate: 3,
              estimated_saving: 30,
            },
          ],
          total_replicas: 2,
          remaining_days: 10,
          estimated_saving: 30,
          currency: 'USD',
          note: 'Preview only.',
        }}
      />,
    )
    expect(screen.getByText('web')).toBeTruthy()
    expect(screen.getByText(/2 replicas . saves/)).toBeTruthy()
    expect(screen.getByText(/changes nothing./)).toBeTruthy()
    expect(screen.getByText(/2 replicas would stop/)).toBeTruthy()
  })

  it('shows the error instead of a preview', () => {
    wrap(
      <PausePreviewDialog
        open
        onOpenChange={vi.fn()}
        pending={false}
        error="usage tracking is not available"
        preview={undefined}
      />,
    )
    expect(screen.getByRole('alert').textContent).toContain('usage tracking')
  })
})
