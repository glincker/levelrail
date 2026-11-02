import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import domainsEn from '../../locales/en/domains.json'
import type { DomainCutover } from '../../queries/domainCutover'
import { CutoverChecklist } from './DomainCutoverCard'
import { createTestQueryClient } from '@/test/testQueryClient'
import { QueryClientProvider } from '@tanstack/react-query'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const ids = [
  'app',
  'certificate',
  'current_dns',
  'switch_dns',
  'authoritative',
  'live',
] as const

function data(over: Partial<DomainCutover>): DomainCutover {
  return {
    domain: 'glinr.com',
    risk: 'likely',
    ready_to_switch: true,
    summary: '',
    proxied: false,
    steps: ids.map((id) => ({
      id,
      state: id === 'app' ? 'pass' : 'wait',
      title: id,
      detail: `detail ${id}`,
    })),
    dns: {
      points: 'previous_server',
      status: 'resolves_elsewhere',
      lower_ttl_to: 300,
    },
    certificate: {
      state: 'off',
      provider_connected: false,
      runbook_href: '/acme-verification-runbook',
    },
    cli: [],
    guide_href: '/move-a-domain-with-no-downtime',
    ...over,
  }
}

function view(d: DomainCutover) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <CutoverChecklist data={d} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('CutoverChecklist', () => {
  it('shows pending certificate wording and downtime risk', () => {
    view(data({}))
    expect(
      screen.getByText(
        'Ready to switch: the app answers on this server for glinr.com (certificate pending)',
      ),
    ).toBeTruthy()
    expect(screen.getByText('Downtime likely')).toBeTruthy()
    expect(screen.getByText(/connected, so HTTP-01 works only/)).toBeTruthy()
  })

  it('offers the one-click certificate when a provider is connected', () => {
    const d = data({})
    d.certificate.provider_connected = true
    d.steps = d.steps.map((s) =>
      s.id === 'certificate'
        ? { ...s, action: { kind: 'request', label: 'x', api: 'POST /x' } }
        : s,
    )
    view(d)
    expect(
      screen.getByRole('button', {
        name: 'Get certificate before switching DNS',
      }),
    ).toBeTruthy()
  })

  it('offers the way back to HTTP-01 once the certificate exists', () => {
    const d = data({})
    d.certificate.challenge = 'dns01'
    d.steps = d.steps.map((s) =>
      s.id === 'certificate'
        ? {
            ...s,
            state: 'pass',
            action: { kind: 'request', label: 'x', api: 'DELETE /x' },
          }
        : s,
    )
    view(d)
    expect(
      screen.getByRole('button', {
        name: 'Switch back to HTTP-01 for this domain',
      }),
    ).toBeTruthy()
    expect(
      screen.queryByRole('button', {
        name: 'Get certificate before switching DNS',
      }),
    ).toBeNull()
  })

  it('is green only when the verdict is none and links the Cloudflare docs when proxied', () => {
    view(data({ risk: 'none', proxied: true, proxy_docs: '/cdn-and-caching' }))
    expect(screen.getByText('No downtime expected')).toBeTruthy()
    expect(
      screen
        .getByRole('link', { name: 'Cloudflare proxy docs' })
        .getAttribute('href'),
    ).toBe('/cdn-and-caching')
  })
})
