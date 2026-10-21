import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { CertTruthBadge } from './CertTruthBadge'
import httpsEn from '../locales/en/https.json'
import type { CertificateStatus } from '../queries/certificates'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['https'],
  defaultNS: 'https',
  resources: { en: { https: httpsEn } },
  interpolation: { escapeValue: false },
})

function cert(over: Partial<CertificateStatus>): CertificateStatus {
  return {
    domain: 'shop.dev',
    not_before: '2026-09-01T00:00:00Z',
    not_after: '2026-12-01T00:00:00Z',
    status: 'healthy',
    source: 'acme',
    ...over,
  }
}

function show(c?: CertificateStatus) {
  render(
    <I18nextProvider i18n={testI18n}>
      <CertTruthBadge cert={c} />
    </I18nextProvider>,
  )
}

describe('CertTruthBadge', () => {
  it('shows an internal certificate without an expiry countdown', () => {
    show(cert({ kind: 'internal', days_left: 0 }))
    expect(screen.getByTestId('cert-truth-badge')).toHaveTextContent(
      'Internal certificate',
    )
    expect(screen.getByTestId('cert-truth-badge')).not.toHaveTextContent('day')
  })

  it('shows staging distinctly', () => {
    show(cert({ kind: 'staging', days_left: 80 }))
    expect(screen.getByTestId('cert-truth-badge')).toHaveTextContent(
      'Staging, 80 days left',
    )
  })

  it('shows Lets Encrypt with days left and the last renewal', () => {
    show(
      cert({
        kind: 'letsencrypt',
        days_left: 54,
        last_renewal: { at: '2026-10-10T00:00:00Z', result: 'ok' },
      }),
    )
    expect(screen.getByTestId('cert-truth-badge')).toHaveTextContent(
      "Let's Encrypt, 54 days left",
    )
    expect(screen.getByText(/Renewed/)).toBeInTheDocument()
  })

  it('shows a failed renewal and when it retries', () => {
    show(
      cert({
        kind: 'letsencrypt',
        days_left: 9,
        last_renewal: {
          at: '2026-10-19T00:00:00Z',
          result: 'failed',
          error: 'boom',
        },
        next_retry_at: '2026-10-20T12:00:00Z',
      }),
    )
    expect(screen.getByText(/Last renewal failed/)).toHaveTextContent(
      /Retrying/,
    )
  })

  it('says there is no certificate', () => {
    show(undefined)
    expect(screen.getByTestId('cert-truth-badge')).toHaveTextContent(
      'No certificate',
    )
  })
})
