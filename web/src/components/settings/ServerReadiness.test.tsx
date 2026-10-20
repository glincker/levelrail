import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type { ServerReadiness as Readiness } from '../../queries/selfUpgrade'
import { ServerReadiness } from './ServerReadiness'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

function renderWith(data: Readiness) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(data),
      }),
    ),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <ServerReadiness />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ServerReadiness', () => {
  it('recommends running behind the proxy that holds the ports', async () => {
    renderWith({
      mode: 'behind_proxy',
      proxy: 'traefik',
      blocked: false,
      summary: 'Ports 80 and 443 are taken by traefik.',
      next_step: 'curl -fsSL x | sudo sh -s -- --coexist',
      checks: [
        {
          id: 'port_80',
          name: 'Port 80',
          status: 'warn',
          detail: 'held by container coolify-proxy',
          fix: 'Install behind it.',
        },
      ],
    })
    expect(
      await screen.findByText(
        'Recommended: install behind your existing proxy',
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByText('held by container coolify-proxy'),
    ).toBeInTheDocument()
    expect(screen.getByText('Install behind it.')).toBeInTheDocument()
    expect(
      screen.getByText('curl -fsSL x | sudo sh -s -- --coexist'),
    ).toBeInTheDocument()
  })

  it('shows a failing check as a fix to make', async () => {
    renderWith({
      mode: 'own_ports',
      blocked: true,
      summary: 'Fix the failing check first: Docker Engine.',
      next_step: 'Upgrade Docker Engine',
      checks: [
        {
          id: 'docker',
          name: 'Docker Engine',
          status: 'fail',
          detail: 'Docker 20.10 is older than the supported 24',
        },
      ],
    })
    expect(await screen.findByText('Fix')).toBeInTheDocument()
    expect(screen.getByText('Docker Engine')).toBeInTheDocument()
  })
})
