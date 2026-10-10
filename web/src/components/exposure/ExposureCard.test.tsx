import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExposureCard } from './ExposureCard'
import exposureEn from '../../locales/en/exposure.json'
import type { ExposureReport } from '../../types/exposure'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['exposure'],
  defaultNS: 'exposure',
  resources: { en: { exposure: exposureEn } },
  interpolation: { escapeValue: false },
})

function report(
  findings: ExposureReport['nodes'][number]['findings'],
): ExposureReport {
  return {
    generated_at: '2026-10-09T00:00:00Z',
    exposed: findings.length,
    high: findings.length,
    nodes: [
      {
        node_id: '',
        node_name: 'local',
        local: true,
        status: 'ok',
        rules_readable: true,
        findings,
      },
    ],
  }
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <ExposureCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

function stub(body: unknown, calls: string[] = []) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push(`${init?.method ?? 'GET'} ${url}`)
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      } as Response)
    }),
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('ExposureCard', () => {
  it('shows a real empty state when nothing is published', async () => {
    stub(report([]))
    renderCard()
    expect(
      await screen.findByText('Nothing of yours is published to the internet'),
    ).toBeInTheDocument()
  })

  it('lists an exposed port with its severity and expands the explanation', async () => {
    const calls: string[] = []
    stub(
      report([
        {
          container: 'typesense',
          image: 'typesense/typesense:27',
          owner: { kind: 'unmanaged' },
          image_kind: 'search',
          protocol: 'tcp',
          host_port: 8108,
          container_port: 8108,
          binds: ['0.0.0.0'],
          class: 'exposed',
          severity: 'high',
          managed: false,
          explanation: 'Port 8108/tcp is published to everyone.',
          can_restrict: true,
          outside_check: { status: 'not_run' },
        },
      ]),
      calls,
    )
    renderCard()
    expect(await screen.findByText('High')).toBeInTheDocument()
    expect(screen.getByText('8108/tcp')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Details/ }))
    expect(
      await screen.findByText('Port 8108/tcp is published to everyone.'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Allow only these sources' }),
    ).toBeInTheDocument()
    expect(calls.filter((c) => c.startsWith('POST'))).toEqual([])
  })
})
