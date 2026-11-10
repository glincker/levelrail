import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AppCapsCard } from './AppCapsCard'
import costsEn from '../../locales/en/costs.json'
import budgetEn from '../../locales/en/budget.json'
import type { SpendCap } from '../../queries/spendCap'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['costs', 'budget'],
  defaultNS: 'costs',
  resources: { en: { costs: costsEn, budget: budgetEn } },
  interpolation: { escapeValue: false },
})

function cap(over: Partial<SpendCap>): SpendCap {
  return {
    app: 'shop',
    guard_mode: 'block',
    monthly_egress_bytes: 0,
    monthly_cost_ceiling: 40,
    thresholds: [80, 100],
    action: 'pause',
    throttle_rps: 0,
    period: '2026-11',
    egress_bytes: 0,
    cost: 10,
    egress_pct: 0,
    cost_pct: 25,
    pct: 25,
    enforced: 'none',
    resets_at: '',
    condition: { healthy: true, reason: '', message: '' },
    cost_priced: true,
    ...over,
  } as SpendCap
}

function renderWith(caps: SpendCap[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Response(JSON.stringify({ caps }), { status: 200 })),
  )
  const qc = new QueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <AppCapsCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('AppCapsCard', () => {
  it('shows the set control and a dry-run button, no override when not blocked', async () => {
    renderWith([cap({})])
    expect(await screen.findByRole('button', { name: 'Set' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Dry run' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Override' })).toBeNull()
  })

  it('shows the egress cap in GiB as an editable field', async () => {
    renderWith([cap({ monthly_egress_bytes: 5 * 1024 ** 3 })])
    const input = await screen.findByLabelText('Egress cap in GiB for shop')
    expect((input as HTMLInputElement).value).toBe('5')
  })

  it('offers an override when deploys are blocked', async () => {
    renderWith([
      cap({
        enforced: 'pause',
        guard: { blocked: true, allowed: false, reasons: [] },
      }),
    ])
    expect(await screen.findByRole('button', { name: 'Override' })).toBeTruthy()
    expect(screen.getByText('Deploys blocked')).toBeTruthy()
  })

  it('shows an active override', async () => {
    renderWith([
      cap({
        guard: {
          blocked: false,
          allowed: true,
          reasons: [],
          override: { reason: 'r', actor: 'a', expires_at: '' },
        },
      }),
    ])
    expect(await screen.findByText('Override active')).toBeTruthy()
  })
})
