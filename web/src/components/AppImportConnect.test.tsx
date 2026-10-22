import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppImportConnect } from './AppImportConnect'
import migrationEn from '../locales/en/migration.json'

function bodyOf(init: RequestInit | undefined): unknown {
  return typeof init?.body === 'string' ? JSON.parse(init.body) : null
}

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

function renderConnect() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <AppImportConnect onCreated={vi.fn()} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AppImportConnect source picker', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('tests the chosen platform without creating a job', async () => {
    const user = userEvent.setup()
    const fn = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            ok: true,
            platform: 'caprover',
            apps: 2,
            databases: 0,
            unsupported: 0,
            servers: [],
            volumes: 0,
            domains: 1,
            crons: 0,
            warnings: [],
          }),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fn)
    renderConnect()

    await user.click(screen.getByRole('button', { name: 'CapRover' }))
    expect(
      screen.getByRole('heading', { name: 'Connect to CapRover' }),
    ).toBeTruthy()
    expect(screen.getByLabelText('Password')).toBeTruthy()
    const test = screen.getByRole('button', { name: 'Test connection' })
    expect((test as HTMLButtonElement).disabled).toBe(true)

    await user.type(
      screen.getByLabelText('CapRover URL'),
      'https://captain.example.com',
    )
    await user.type(screen.getByLabelText('Password'), 'fake-password')
    await user.click(test)

    expect(await screen.findByText(/Connected to CapRover/)).toBeTruthy()
    const [url, init] = fn.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/migration/apps/test')
    expect(bodyOf(init)).toMatchObject({
      platform: 'caprover',
      url: 'https://captain.example.com',
    })
    expect(fn).toHaveBeenCalledTimes(1)
  })
})
