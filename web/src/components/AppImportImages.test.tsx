import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppImportImages } from './AppImportImages'
import migrationEn from '../locales/en/migration.json'
import type {
  AppImportImages as Images,
  AppImportSession,
} from '../queries/appImport'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

const session = { id: 'appimp-1' } as AppImportSession

const listed: Images = {
  running: false,
  credentials_held: false,
  supported: true,
  max_bytes: 1 << 30,
  images: [
    {
      source_id: 'a',
      app: 'web',
      image: 'webimg:1',
      source_image_id: 'sha256:' + 'a'.repeat(64),
      state: 'failed',
      bytes: 2048,
      verified: false,
      error: 'ssh login to root@old:22 failed',
    },
    {
      source_id: 'b',
      app: 'api',
      image: 'apiimg:1',
      source_image_id: 'sha256:' + 'b'.repeat(64),
      loaded_image_id: 'sha256:' + 'b'.repeat(64),
      state: 'verified',
      bytes: 4096,
      verified: true,
    },
  ],
}

function stubFetch(responses: Record<string, unknown>) {
  const fn = vi.fn((url: string) => {
    const key = Object.keys(responses).find((k) => url.endsWith(k)) ?? ''
    return Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(responses[key]),
    } as unknown as Response)
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderStep() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <AppImportImages session={session} onBack={vi.fn()} onNext={vi.fn()} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AppImportImages', () => {
  it('lists each image with its state and error', async () => {
    stubFetch({ '/images': listed })
    renderStep()
    expect(await screen.findByText('webimg:1')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('Verified')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('ssh login')
    expect(
      screen.getByText('1 image(s) not on this node yet'),
    ).toBeInTheDocument()
  })

  it('sends the login and key, then clears the key field', async () => {
    const fetchMock = stubFetch({
      '/images': listed,
      '/images/transfer': { ...listed, running: true, source: 'root@old:22' },
    })
    renderStep()
    const user = userEvent.setup()
    await user.type(
      await screen.findByLabelText('Source SSH login'),
      'root@old',
    )
    const key = screen.getByLabelText('Private key')
    await user.type(key, 'KEY')
    await user.click(screen.getByRole('button', { name: /Move remaining/ }))
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/migration/apps/sessions/appimp-1/images/transfer',
        expect.objectContaining({ method: 'POST' }),
      ),
    )
    const call = fetchMock.mock.calls.find(([u]) => u.endsWith('/transfer'))
    const init = (call as unknown as [string, RequestInit])[1]
    expect(JSON.parse(init.body as string)).toEqual({
      ssh: 'root@old',
      private_key: 'KEY',
    })
    await waitFor(() => expect(key).toHaveValue(''))
    expect(await screen.findByRole('button', { name: /Cancel/ })).toBeEnabled()
  })

  it('says when no app needs an image moved', async () => {
    stubFetch({ '/images': { ...listed, images: [] } })
    renderStep()
    expect(
      await screen.findByText(/No selected app uses an image/),
    ).toBeInTheDocument()
  })
})
