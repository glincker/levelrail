import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../../locales/en/updates.json'
import { AutoUpdatesCard } from './AutoUpdatesCard'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const policy = {
  mode: 'patch',
  effective: 'patch',
  window_cron: '0 2 * * *',
  window_duration_minutes: 180,
  timezone: 'UTC',
  min_release_age_days: 0,
  notify_channel_ids: [],
  paused: false,
  server_timezone: 'UTC',
  next_window: null,
}
const settings = {
  channel: 'stable',
  auto_update_enabled: true,
  offered_channels: ['stable'],
}

function mockFetch(initial: typeof policy) {
  const puts: unknown[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      if (url.includes('/updates/auto')) {
        if (init?.method === 'PUT') {
          const body = JSON.parse(init.body as string) as typeof policy
          puts.push(body)
          return Promise.resolve(Response.json({ ...initial, ...body }))
        }
        return Promise.resolve(Response.json(initial))
      }
      return Promise.resolve(Response.json(settings))
    }),
  )
  return puts
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <AutoUpdatesCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AutoUpdatesCard', () => {
  it('states the night window in plain words and hides the schedule when off', async () => {
    mockFetch({ ...policy, mode: 'off' })
    renderCard()
    expect(await screen.findByText(/never installed on their own/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Schedule/ })).toBeNull()
  })

  it('saves the master switch immediately', async () => {
    const puts = mockFetch(policy)
    renderCard()
    expect(
      await screen.findByText(/between 02:00 and 05:00, every night/),
    ).toBeTruthy()
    fireEvent.click(screen.getByRole('switch', { name: 'Automatic updates' }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect((puts[0] as { mode: string }).mode).toBe('off')
  })

  it('rejects an invalid time zone and keeps save disabled', async () => {
    mockFetch(policy)
    renderCard()
    fireEvent.click(await screen.findByRole('button', { name: /Schedule/ }))
    const zone = await screen.findByLabelText('Time zone')
    fireEvent.change(zone, { target: { value: 'Mars/Base' } })
    expect(screen.getByRole('alert').textContent).toContain('valid time zone')
    expect(
      screen.getByRole('button', { name: 'Save schedule' }),
    ).toHaveProperty('disabled', true)
  })
})
