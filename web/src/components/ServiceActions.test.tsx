import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../locales/en/appServices.json'
import { ServiceActions } from './ServiceActions'
import type { AppDetail } from '../types/appDetail'

vi.mock('../hooks/useAbilities', () => ({
  useAbilities: () => ({ role: 'admin', can: () => true }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['appServices'],
  defaultNS: 'appServices',
  resources: { en: { appServices: en } },
  interpolation: { escapeValue: false },
})

const svc = { name: 'shop-web', image: 'x', port: 80, replicas: 1 } as AppDetail

function mockFetch(putStatus: number) {
  const calls: { url: string; method: string; body?: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({
        url:
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.href
              : input.url,
        method,
        body: init?.body as string,
      })
      if (method === 'PUT' && putStatus !== 200) {
        return Promise.resolve(
          new Response(JSON.stringify({ error: 'replicas too high' }), {
            status: putStatus,
          }),
        )
      }
      return Promise.resolve(Response.json(svc))
    }),
  )
  return calls
}

function renderIt() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <ServiceActions svc={svc} appName="shop" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('ServiceActions', () => {
  it('applies only the changed service and shows server errors inline', async () => {
    const calls = mockFetch(400)
    renderIt()
    const apply = screen.getByRole('button', { name: 'Apply' })
    expect(apply).toBeDisabled()
    await userEvent.click(
      screen.getByRole('button', { name: 'Increase replicas for shop-web' }),
    )
    await userEvent.click(apply)
    const put = await waitFor(() => {
      const c = calls.find((x) => x.method === 'PUT')
      expect(c).toBeDefined()
      return c
    })
    expect(put?.url).toBe('/api/v1/apps/shop-web')
    expect(
      (JSON.parse(put?.body ?? '{}') as { replicas: number }).replicas,
    ).toBe(2)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'replicas too high',
    )
  })

  it('restarts only after confirm', async () => {
    const calls = mockFetch(200)
    renderIt()
    await userEvent.click(
      screen.getByRole('button', { name: 'Restart shop-web' }),
    )
    expect(calls.some((c) => c.url.endsWith('/restart'))).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === 'POST' && c.url === '/api/v1/apps/shop-web/restart',
        ),
      ).toBe(true),
    )
  })
})
