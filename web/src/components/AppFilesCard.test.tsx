import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import appFilesEn from '../locales/en/appFiles.json'
import { AppFilesCard } from './AppFilesCard'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['appFiles'],
  defaultNS: 'appFiles',
  resources: { en: { appFiles: appFilesEn } },
  interpolation: { escapeValue: false },
})

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const FILES = {
  app_name: 'demo',
  files: [
    {
      path: '/etc/app/config.yaml',
      mode: '0644',
      secret: false,
      redacted: false,
      size_bytes: 5,
      content: 'a: 1\n',
    },
    {
      path: '/etc/app/token',
      mode: '0600',
      secret: true,
      redacted: true,
      size_bytes: 12,
    },
  ],
}

function stubFetch() {
  const fn = vi.fn<
    (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
  >((_input, init) =>
    Promise.resolve(
      init?.method === 'PUT' ? jsonResponse({}) : jsonResponse(FILES),
    ),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderCard() {
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={testI18n}>
        <AppFilesCard appName="demo" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('AppFilesCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists path, mode and a Redacted badge for secret files', async () => {
    stubFetch()
    renderCard()
    expect(await screen.findByText('/etc/app/config.yaml')).toBeInTheDocument()
    expect(screen.getByText('0644')).toBeInTheDocument()
    expect(screen.getByText('/etc/app/token')).toBeInTheDocument()
    expect(screen.getByText('0600')).toBeInTheDocument()
    expect(screen.getAllByText('Redacted')).toHaveLength(1)
  })

  it('submits the PUT from the add dialog', async () => {
    const fetchMock = stubFetch()
    const user = userEvent.setup()
    renderCard()
    await screen.findByText('/etc/app/token')
    await user.click(screen.getByRole('button', { name: 'Add file' }))
    await user.type(await screen.findByLabelText('Path'), '/etc/new.conf')
    await user.type(screen.getByLabelText('Content'), 'hello')
    await user.click(screen.getByRole('button', { name: 'Save file' }))

    await waitFor(() => {
      const put = fetchMock.mock.calls.find(([, i]) => i?.method === 'PUT')
      expect(put).toBeDefined()
      expect(put?.[0]).toBe('/api/v1/apps/demo/files')
      expect(JSON.parse(put?.[1]?.body as string)).toEqual({
        path: '/etc/new.conf',
        content: 'hello',
        mode: '0644',
        secret: false,
      })
    })
  })
})
