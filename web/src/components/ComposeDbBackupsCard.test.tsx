import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposeDbBackupsCard } from './ComposeDbBackupsCard'
import backupsEn from '../locales/en/backups.json'
import commonEn from '../locales/en/common.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({ can: () => true }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'backups'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, backups: backupsEn } },
  interpolation: { escapeValue: false },
})

const service = {
  service: 'db',
  engine: 'postgres',
  image: 'postgres:16',
  scheduled: true,
  schedule: '30 2 * * *',
  target_id: 't1',
  retain: 5,
  retain_days: 0,
  enabled: true,
}

function json(body: unknown, status = 200): Promise<Response> {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function bodyOf(init?: RequestInit): string {
  return typeof init?.body === 'string' ? init.body : ''
}

function mount() {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const u = url
    if (u.endsWith('/backup-targets')) {
      return json([{ id: 't1', name: 'bucket', provider: 'aws', bucket: 'b' }])
    }
    if (u.endsWith('/compose-db-services')) return json([service])
    if (init?.method === 'POST')
      return json({ id: 'r1', status: 'running' }, 202)
    if (init?.method === 'PUT') return json({})
    return json([])
  })
  vi.stubGlobal('fetch', fetchMock)
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <ComposeDbBackupsCard appName="shop" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ComposeDbBackupsCard', () => {
  it('renders the service, saves a schedule and triggers a backup', async () => {
    const fetchMock = mount()
    expect(await screen.findByText('postgres')).toBeTruthy()

    fireEvent.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => {
      const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT')
      expect(put).toBeTruthy()
      expect(put?.[0]).toBe(
        '/api/v1/apps/shop/compose-db-services/db/backup-schedule',
      )
      expect(JSON.parse(bodyOf(put?.[1]))).toEqual({
        target_id: 't1',
        schedule: '30 2 * * *',
        retain: 5,
        retain_days: 0,
        enabled: true,
      })
    })

    fireEvent.click(screen.getByRole('button', { name: 'Back up now' }))
    await waitFor(() => {
      const post = fetchMock.mock.calls.find((c) => c[1]?.method === 'POST')
      expect(JSON.parse(bodyOf(post?.[1]))).toEqual({ target_id: 't1' })
    })
  })
})
