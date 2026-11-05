import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import domainsEn from '../../locales/en/domains.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import type { DomainChange } from '../../queries/domainChanges'
import { DomainRollbackStrip } from './DomainRollbackStrip'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['domains', 'common'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const MIN = 60_000

function change(over: Partial<DomainChange>): DomainChange {
  return {
    run_id: 'run1',
    kind: 'dns',
    created_at: new Date(Date.now() - MIN).toISOString(),
    summary: 'Added 2 records',
    rollback_until: new Date(Date.now() + 42 * MIN + 1000).toISOString(),
    can_rollback: true,
    already_undone: false,
    ...over,
  }
}

let posts = 0

function mockApi(changes: DomainChange[]) {
  posts = 0
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        posts += 1
        return Promise.resolve(
          new Response(
            JSON.stringify({
              results: [
                { name: 'a', type: 'A', outcome: 'removed' },
                { name: 'b', type: 'TXT', outcome: 'skipped' },
              ],
            }),
          ),
        )
      }
      expect(url).toContain('/changes')
      return Promise.resolve(new Response(JSON.stringify({ changes })))
    }),
  )
}

function renderStrip() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <DomainRollbackStrip domain="glinr.com" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('DomainRollbackStrip', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders nothing without changes', async () => {
    mockApi([])
    const { container } = renderStrip()
    await waitFor(() => {
      expect(fetch).toHaveBeenCalled()
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the countdown in plain words', async () => {
    mockApi([change({})])
    renderStrip()
    expect(
      await screen.findByText('Can be rolled back for 43 more minutes'),
    ).toBeInTheDocument()
  })

  it('confirms once and shows per-record outcomes', async () => {
    mockApi([change({})])
    renderStrip()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Roll back this change' }),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Roll back' }))
    expect(
      await screen.findByText('Changed since, left alone'),
    ).toBeInTheDocument()
    expect(posts).toBe(1)
  })

  it('shows a muted closed state', async () => {
    mockApi([
      change({
        can_rollback: false,
        rollback_until: new Date(Date.now() - MIN).toISOString(),
      }),
    ])
    renderStrip()
    expect(
      await screen.findByText('Rollback window closed'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Roll back this change' }),
    ).toBeNull()
  })
})
