import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import securityEn from '../../locales/en/security.json'
import type { PostureItem } from '../../queries/securityCenter'

const push = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useRouter: () => ({ history: { push } }) }
})
vi.mock('../../hooks/useBrand', () => ({
  useBrand: () => ({ BinaryName: 'testplatform' }),
}))

import { PostureChecklist } from './PostureOverview'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['security'],
  defaultNS: 'security',
  resources: { en: { security: securityEn } },
  interpolation: { escapeValue: false },
})

const calls: { url: string; method: string; body: string }[] = []

function mockFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      calls.push({
        url: input,
        method: init?.method ?? 'GET',
        body: typeof init?.body === 'string' ? init.body : '',
      })
      return Promise.resolve(
        new Response('{}', {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

function renderList(items: PostureItem[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <PostureChecklist
          title="Platform"
          items={items}
          onReviewSessions={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

const items: PostureItem[] = [
  {
    id: 'approval_password_only',
    severity: 'low',
    status: 'fail',
    count: 0,
    fix: {
      kind: 'action',
      action: 'set_policy',
      params: { approval_scope: 'all_methods' },
    },
  },
  {
    id: 'public_exposure',
    severity: 'high',
    status: 'fail',
    count: 2,
    fix: { kind: 'link', link: '/settings/firewall', cli: 'firewall exposure' },
  },
  { id: 'token_root', severity: 'high', status: 'pass', count: 0 },
  { id: 'brand_new_rule', severity: 'medium', status: 'unknown', count: 0 },
]

describe('PostureChecklist', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    calls.length = 0
    push.mockReset()
  })

  it('lists failing and unknown checks first and hides passing ones', async () => {
    mockFetch()
    const user = userEvent.setup()
    renderList(items)
    expect(
      screen.getByText('Container ports open to the internet'),
    ).toBeInTheDocument()
    expect(screen.getByText('2 affected')).toBeInTheDocument()
    expect(
      screen.getByText(/testplatform-cli firewall exposure/),
    ).toBeInTheDocument()
    expect(screen.getByText('brand_new_rule')).toBeInTheDocument()
    expect(
      screen.queryByText('No API token holds root'),
    ).not.toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Show passing checks' }),
    )
    expect(screen.getByText('No API token holds root')).toBeInTheDocument()
  })

  it('applies a one click policy fix', async () => {
    mockFetch()
    const user = userEvent.setup()
    renderList(items)
    await user.click(
      screen.getByRole('button', { name: 'Apply the recommended policy' }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.method === 'PUT')).toBe(true)
    })
    const put = calls.find((c) => c.method === 'PUT')
    expect(put?.url).toBe('/api/v1/security/policy')
    expect(JSON.parse(put?.body ?? '{}')).toEqual({
      approval_scope: 'all_methods',
    })
  })

  it('opens a link fix inside the dashboard', async () => {
    mockFetch()
    const user = userEvent.setup()
    renderList(items)
    await user.click(screen.getByRole('button', { name: 'Open' }))
    expect(push).toHaveBeenCalledWith('/settings/firewall')
  })
})
