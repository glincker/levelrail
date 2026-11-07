import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AppNextCacheRow } from './AppNextCacheRow'
import resourcesEn from '../locales/en/resources.json'
import type { AppDetail } from '../types/appDetail'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['resources'],
  defaultNS: 'resources',
  resources: { en: { resources: resourcesEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function app(extra: Partial<AppDetail> = {}): AppDetail {
  return {
    name: 'web',
    image: 'x:1',
    port: 3000,
    strategy: 'rolling',
    replicas: 1,
    suspended: false,
    env_dirty: false,
    ...extra,
  } as AppDetail
}

function mockApi(dbs: unknown[], puts: string[] = []) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === 'PUT') {
        puts.push(typeof init.body === 'string' ? init.body : '')
        return Promise.resolve(json(app({ next_cache: 'redis' })))
      }
      if (url.startsWith('/api/v1/databases')) return Promise.resolve(json(dbs))
      return Promise.resolve(json({ error: 'none' }, 404))
    }),
  )
}

function renderRow(a: AppDetail) {
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <AppNextCacheRow app={a} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AppNextCacheRow', () => {
  it('sends next_cache redis when the toggle is switched on', async () => {
    const puts: string[] = []
    mockApi([], puts)
    renderRow(app())
    await userEvent.click(screen.getByRole('switch'))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(JSON.parse(puts[0] ?? '{}')).toMatchObject({ next_cache: 'redis' })
  })

  it('shows the linked Redis database', async () => {
    mockApi([{ name: 'cache', engine: 'redis' }])
    renderRow(
      app({
        next_cache: 'redis',
        database_env: { REDIS_URL: { database: 'cache', field: 'url' } },
      }),
    )
    expect(await screen.findByText('Redis database linked: cache')).toBeTruthy()
  })

  it('warns when no Redis database is linked and offers to create one', async () => {
    mockApi([])
    renderRow(app({ next_cache: 'redis' }))
    expect(await screen.findByText(/No Redis database is linked/)).toBeTruthy()
    expect(screen.getByText('Create Redis')).toBeTruthy()
  })

  it('suggests the cache when replicas exceed one and it is off', () => {
    mockApi([])
    renderRow(app({ replicas: 3 }))
    expect(
      screen.getByText(/runs 3 replicas without a shared cache/),
    ).toBeTruthy()
  })
})
