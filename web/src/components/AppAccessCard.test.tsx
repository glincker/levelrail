import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppAccessCard } from './AppAccessCard'
import accessEn from '../locales/en/appAccess.json'
import type { AppUrlAddress, AppUrls } from '../queries/appUrls'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['appAccess'],
  defaultNS: 'appAccess',
  resources: { en: { appAccess: accessEn } },
  interpolation: { escapeValue: false },
})

const RAW = 'read tcp 127.0.0.1:1->127.0.0.1:2: connection reset by peer'
const hint = 'reachable' as const
const loop: AppUrlAddress = {
  kind: 'loopback',
  host: '127.0.0.1',
  url: 'http://127.0.0.1:2000',
  reachable_hint: hint,
}
const lan: AppUrlAddress = {
  kind: 'lan',
  host: '10.181.0.1',
  url: 'http://10.181.0.1:2000',
  reachable_hint: hint,
}
const pub: AppUrlAddress = {
  kind: 'public',
  host: '72.61.8.70',
  url: 'http://72.61.8.70:2000',
  reachable_hint: hint,
}
const auto: AppUrlAddress = {
  kind: 'domain',
  host: 'a.sslip.io',
  url: 'https://a.sslip.io',
  reachable_hint: 'unknown',
}
const custom: AppUrlAddress = {
  kind: 'domain',
  host: 'app.example.com',
  url: 'https://app.example.com',
  reachable_hint: 'unknown',
}

function mockApi(addresses: AppUrlAddress[], failing = false) {
  const urls: AppUrls = {
    app: 'a',
    container_port: 3000,
    host_port: 2000,
    bind_address: 'public',
    running: true,
    local_node: true,
    addresses,
  }
  const targets = failing
    ? [
        {
          kind: 'container',
          url: 'http://127.0.0.1:2000/api/health',
          ok: false,
          failure: 'container_not_listening',
          detail: RAW,
        },
      ]
    : []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string) => {
      const body = input.endsWith('/check')
        ? { app: 'a', scope: 'x', targets }
        : urls
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

function renderCard() {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <AppAccessCard appName="a" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AppAccessCard', () => {
  it('shows the custom domain as the hero and keeps advanced collapsed', async () => {
    mockApi([loop, lan, pub, auto, custom])
    renderCard()
    const hero = await screen.findByRole('region', { name: 'Open your app' })
    expect(hero.textContent).toContain('https://app.example.com')
    expect(
      screen
        .getByRole('button', { name: /Advanced/ })
        .getAttribute('aria-expanded'),
    ).toBe('false')
    expect(screen.queryByText('Container port')).toBeNull()
  })

  it('falls back to the automatic domain, then the public ip', async () => {
    mockApi([loop, lan, pub, auto])
    const first = renderCard()
    expect(
      (await screen.findByRole('region', { name: 'Open your app' }))
        .textContent,
    ).toContain('https://a.sslip.io')
    first.unmount()
    mockApi([loop, lan, pub])
    renderCard()
    await waitFor(() =>
      expect(
        screen.getByRole('region', { name: 'Open your app' }).textContent,
      ).toContain('http://72.61.8.70:2000'),
    )
  })

  it('expands advanced and promotes the private address with no public address', async () => {
    mockApi([loop, lan])
    renderCard()
    const hero = await screen.findByRole('region', { name: 'Open your app' })
    expect(hero.textContent).toContain('http://10.181.0.1:2000')
    expect(
      screen
        .getByRole('button', { name: /Advanced/ })
        .getAttribute('aria-expanded'),
    ).toBe('true')
    expect(screen.getByText('Container port')).toBeTruthy()
  })

  it('hides the raw error until Show details', async () => {
    mockApi([loop, pub], true)
    renderCard()
    const toggle = await screen.findByRole('button', { name: 'Show details' })
    expect(
      screen.getByRole('region', { name: 'Open your app' }).textContent,
    ).toContain('Not answering')
    expect(screen.queryByText(RAW)).toBeNull()
    await userEvent.click(toggle)
    expect(screen.getByText(RAW)).toBeTruthy()
  })
})
