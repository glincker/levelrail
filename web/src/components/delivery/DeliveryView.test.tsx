import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import deliveryEn from '../../locales/en/delivery.json'
import commonEn from '../../locales/en/common.json'
import { DeliveryView } from './DeliveryView'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['delivery', 'common'],
  defaultNS: 'delivery',
  resources: { en: { delivery: deliveryEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return {
    ok: status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const failedStatus = {
  id: 'cs1',
  at: '2026-11-15T10:00:00Z',
  kind: 'commit_status',
  status: 'failed',
  actor: 'pipeline',
  title: 'levelrail/ci failure on abc1234def',
  detail: 'github; Build failed; 403 forbidden token ***',
  url: 'https://example.com/run/1',
  ref: { type: 'commit_status', id: 'cs1' },
}
const oldDeploy = {
  id: 'dpl_1',
  at: '2026-11-01T10:00:00Z',
  kind: 'deploy',
  status: 'succeeded',
  actor: 'user',
  title: 'Deployed v1',
}

function mockApi(pages: Record<string, unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string) => {
      const url = input
      if (url.includes('/bindings'))
        return Promise.resolve(json({ bindings: [] }))
      if (url.startsWith('/api/v1/connections'))
        return Promise.resolve(json({ connections: null }))
      const before = new URL(url, 'http://x').searchParams.get('before') ?? ''
      return Promise.resolve(json(pages[before] ?? { items: null }))
    }),
  )
}

function renderView() {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <DeliveryView app="web" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('DeliveryView', () => {
  it('renders a failed commit status with a collapsed masked error and a warning', async () => {
    mockApi({ '': { items: [failedStatus] } })
    renderView()
    expect(await screen.findByText(failedStatus.title)).toBeInTheDocument()
    expect(screen.getByText('abc1234')).toBeInTheDocument()
    expect(screen.getByText('Build failed')).toBeInTheDocument()
    expect(screen.getByText('Post to the forge failed')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent(/failing/i)
    expect(screen.getByRole('link', { name: /open run/i })).toHaveAttribute(
      'href',
      'https://example.com/run/1',
    )
    const details = screen
      .getByText('Post to the forge failed')
      .closest('details')
    expect(details).not.toHaveAttribute('open')
  })

  it('loads the next page with the cursor', async () => {
    mockApi({
      '': { items: [failedStatus], next_cursor: 'c2' },
      c2: { items: [oldDeploy] },
    })
    renderView()
    await screen.findByText(failedStatus.title)
    await userEvent.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await screen.findByText('Deployed v1')).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull(),
    )
  })

  it('filters by status and shows the no-match state', async () => {
    mockApi({ '': { items: [oldDeploy] } })
    renderView()
    await screen.findByText('Deployed v1')
    await userEvent.selectOptions(screen.getByRole('combobox'), 'failed')
    expect(
      await screen.findByText('Nothing matches these filters'),
    ).toBeInTheDocument()
  })

  it('shows the empty state for a null items array', async () => {
    mockApi({ '': { items: null } })
    renderView()
    expect(
      await screen.findByText('No delivery activity yet'),
    ).toBeInTheDocument()
  })
})
