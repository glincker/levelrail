import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import getStartedEn from '../../locales/en/getStarted.json'
import { FirstDeploy } from './FirstDeploy'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../CreateResourceWizard', () => ({
  CreateResourceWizard: ({ trigger }: { trigger: ReactNode }) => <>{trigger}</>,
}))
vi.mock('../EnableHttpsCard', () => ({
  EnableHttpsCard: () => <div>https card</div>,
}))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['getStarted'],
  defaultNS: 'getStarted',
  resources: { en: { getStarted: getStartedEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function stubFetch(createStatus: number) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url.includes('/settings/ingress/https')) {
      return Promise.resolve(
        json({
          state: 'off',
          staging: false,
          suggested_domain: '1-2-3-4.sslip.io',
        }),
      )
    }
    if (init?.method === 'POST') {
      return Promise.resolve(
        createStatus === 200
          ? json({ name: 'sample-app' })
          : json({ error: 'boom' }, createStatus),
      )
    }
    return Promise.resolve(json([]))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderIt(onStarted = vi.fn()) {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <FirstDeploy onStarted={onStarted} onDone={vi.fn()} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return onStarted
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('FirstDeploy', () => {
  it('shows the zero-config address and the HTTPS action before any deploy', async () => {
    stubFetch(200)
    renderIt()
    expect(
      await screen.findByText(/http:\/\/sample-app\.1-2-3-4\.sslip\.io/),
    ).toBeTruthy()
    expect(screen.getByText('https card')).toBeTruthy()
  })

  it('keeps import and git reachable as secondary links', () => {
    stubFetch(200)
    renderIt()
    expect(
      screen.getByRole('link', { name: /Import it/ }).getAttribute('href'),
    ).toBe('/settings/import-platform')
    expect(
      screen.getByRole('link', { name: /Connect GitHub/ }).getAttribute('href'),
    ).toBe('/settings/github-app')
  })

  it('tracks the app after the sample deploys', async () => {
    stubFetch(200)
    const onStarted = renderIt()
    fireEvent.click(screen.getByRole('button', { name: 'Deploy sample app' }))
    await waitFor(() => {
      expect(onStarted).toHaveBeenCalledTimes(1)
    })
  })

  it('treats an existing sample app as started', async () => {
    stubFetch(409)
    const onStarted = renderIt()
    fireEvent.click(screen.getByRole('button', { name: 'Deploy sample app' }))
    await waitFor(() => {
      expect(onStarted).toHaveBeenCalledTimes(1)
    })
  })

  it('offers a retry with the error when creation fails', async () => {
    stubFetch(500)
    renderIt()
    fireEvent.click(screen.getByRole('button', { name: 'Deploy sample app' }))
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy()
  })

  it('keeps all three paths reachable in a compact row with 44px touch targets', () => {
    stubFetch(200)
    renderIt()
    for (const name of [
      'Deploy sample app',
      'Browse templates',
      'Deploy a repo',
    ]) {
      const button = screen.getByRole('button', { name })
      expect(button.parentElement?.className).toContain('[&_button]:min-h-11')
      expect(button.closest('[class*="items-center"]')).not.toBeNull()
    }
  })
})
