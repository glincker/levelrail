import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { CertificateCenterTable } from './CertificateCenterTable'
import settingsEn from '../locales/en/settings.json'
import type { CertificateCenterRow } from '../lib/certificateCenter'
import type { CertificateStatus } from '../queries/certificates'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('./DomainTLSCertControl', () => ({ DomainTLSCertControl: () => null }))
vi.mock('./RenewCertificateButton', () => ({
  RenewCertificateButton: () => null,
}))

const day = 86_400_000
function row(domain: string, inDays: number): CertificateCenterRow {
  return {
    domain,
    appName: 'web',
    cert: {
      domain,
      source: 'acme',
      status: 'healthy',
      not_after: new Date(Date.now() + inDays * day).toISOString(),
    } as unknown as CertificateStatus,
  }
}

afterEach(() => vi.unstubAllGlobals())

describe('CertificateCenterTable', () => {
  it('keeps rows expiring within 14 days and bulk renew posts their hosts', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ had_stored_certificate: true }), {
          status: 202,
        }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const i18n = i18next.createInstance()
    void i18n.use(initReactI18next).init({
      lng: 'en',
      resources: { en: { settings: settingsEn } },
      interpolation: { escapeValue: false },
    })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={i18n}>
          <CertificateCenterTable
            rows={[row('soon.example.com', 5), row('later.example.com', 60)]}
          />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByLabelText('Expires within'))
    expect(screen.queryByText('later.example.com')).toBeNull()
    await userEvent.click(screen.getByLabelText('Select soon.example.com'))
    await userEvent.click(
      screen.getByRole('button', { name: 'Renew 1 selected' }),
    )
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    const call = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(call[0]).toContain('/domains/soon.example.com/cert/renew')
    expect(call[1].method).toBe('POST')
  })
})
