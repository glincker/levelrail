import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import stacksEn from '../../locales/en/stacks.json'
import type { Stack, StackRevisionSummary } from '../../types/stacks'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'stacks'],
  defaultNS: 'stacks',
  resources: { en: { common: commonEn, stacks: stacksEn } },
  interpolation: { escapeValue: false },
})

export function stack(overrides: Partial<Stack> = {}): Stack {
  return {
    name: 'shop',
    project_id: 'proj_1',
    project: 'main',
    allow_prune: false,
    drift_policy: 'warn',
    require_approval: false,
    created_at: '2026-10-20T00:00:00Z',
    created_by: 'ada@example.com',
    latest_revision: 3,
    latest_status: 'applied',
    ...overrides,
  }
}

export function revision(
  number: number,
  overrides: Partial<StackRevisionSummary> = {},
): StackRevisionSummary {
  return {
    number,
    status: 'applied',
    plan_hash: `hash${String(number)}abcdef0123`,
    actor: 'ada@example.com',
    created_at: '2026-10-21T00:00:00Z',
    ...overrides,
  }
}

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status })
}

export function renderWithProviders(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>{ui}</I18nextProvider>
    </QueryClientProvider>,
  )
}
