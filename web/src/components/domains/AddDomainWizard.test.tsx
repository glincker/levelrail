import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { AddDomainWizard } from './AddDomainWizard'
import { classifyDomain, normalizeDomainInput } from '../../lib/domainWizard'
import domainsEn from '../../locales/en/domains.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function res(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderWizard() {
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <Dialog open>
          <DialogContent>
            <AddDomainWizard
              apps={[{ name: 'web' }]}
              claimedBy={new Map([['taken.com', 'other']])}
              onClose={() => undefined}
              onAdvanced={() => undefined}
            />
          </DialogContent>
        </Dialog>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('domain wizard helpers', () => {
  it('classifies and normalizes what people paste', () => {
    expect(normalizeDomainInput(' HTTPS://Example.com/path ')).toBe(
      'example.com',
    )
    expect(classifyDomain('example.com')).toBe('apex')
    expect(classifyDomain('www.example.com')).toBe('www')
    expect(classifyDomain('app.example.com')).toBe('subdomain')
    expect(classifyDomain('*.example.com')).toBe('wildcard')
    expect(classifyDomain('nope')).toBe('invalid')
    expect(classifyDomain('')).toBe('empty')
  })
})

describe('AddDomainWizard', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('explains a bad or taken domain in a plain sentence', async () => {
    const user = userEvent.setup()
    renderWizard()
    const box = screen.getByRole('textbox', { name: 'Your domain' })
    await user.type(box, 'nope')
    await user.tab()
    expect(screen.getByText(/does not look like a domain/)).toBeInTheDocument()
    await user.clear(box)
    await user.type(box, 'taken.com')
    await user.tab()
    expect(screen.getByText(/app other already uses/)).toBeInTheDocument()
  })

  it('adds the domain, moves to step 2 and survives a reload', async () => {
    const fetchMock = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit): Promise<Response> =>
        Promise.resolve(
          init?.method === 'PATCH'
            ? res({ app: 'web', domains: ['shop.example.com'], changed: true })
            : res({
                domain: 'shop.example.com',
                status: 'not_resolving',
                expected_host: 'srv.example.net',
                dns_provider: 'none',
              }),
        ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    const first = renderWizard()
    await user.type(
      screen.getByRole('textbox', { name: 'Your domain' }),
      'shop.example.com',
    )
    expect(screen.getByText(/subdomain, it needs one record/)).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => {
      expect(screen.getByText('Point your domain here')).toBeInTheDocument()
    })
    expect(await screen.findByText('srv.example.net')).toBeInTheDocument()
    expect(screen.getByText(/No DNS provider is connected/)).toBeInTheDocument()

    first.unmount()
    renderWizard()
    expect(screen.getByText('Point your domain here')).toBeInTheDocument()
  })
})
