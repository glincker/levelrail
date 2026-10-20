import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { HostFirewallCard } from './HostFirewallCard'
import settingsEn from '../locales/en/settings.json'
import type { HostFirewall } from '../types/hostFirewall'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const off: HostFirewall = {
  installed: true,
  active: false,
  required: [
    { port: 22, protocol: 'tcp' },
    { port: 443, protocol: 'udp' },
  ],
  commands: ['ufw allow 22/tcp', 'ufw allow 443/udp', 'ufw --force enable'],
}

function json(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as Response
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <HostFirewallCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('HostFirewallCard', () => {
  it('previews the exact commands and only enables after confirming', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push(`${init?.method ?? 'GET'} ${url}`)
        return Promise.resolve(json(off))
      }),
    )
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Turn on firewall' }),
    )
    expect(await screen.findByText(/ufw allow 22\/tcp/)).toBeInTheDocument()
    expect(calls.filter((c) => c.startsWith('POST'))).toEqual([])
    expect(calls.filter((c) => c.startsWith('GET'))).toHaveLength(1)

    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() =>
      expect(calls).toContain('POST /api/v1/firewall/host/enable'),
    )
  })

  it('explains when ufw is not installed and offers no switch', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(json({ ...off, installed: false }))),
    )
    renderCard()
    expect(await screen.findByText('ufw not installed')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Turn on firewall' }),
    ).toBeNull()
  })
})
