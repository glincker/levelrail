import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { RuntimeOptionsCard } from './RuntimeOptionsCard'
import en from '../../locales/en/runtimeOptions.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('../../hooks/useAbilities', () => ({
  useAbilities: () => ({ role: 'admin', can: () => true }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['runtimeOptions'],
  defaultNS: 'runtimeOptions',
  resources: { en: { runtimeOptions: en } },
  interpolation: { escapeValue: false },
})

const resource = {
  app: 'web',
  service: 'web',
  revision: 'abc123',
  runtime: {},
  editable: ['tmpfs', 'ulimits', 'init'],
  bounds: {
    tmpfs_max: 8,
    tmpfs_bytes_max: 1,
    extra_hosts_max: 8,
    dns_max: 3,
    stop_timeout_max_seconds: 600,
    pids_limit_max: 32768,
    ulimit_count_max: 8,
    ulimit_bytes_max: 1,
    ulimits: ['nofile'],
    sysctls: [],
  },
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('RuntimeOptionsCard', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  function mount() {
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <RuntimeOptionsCard appName="web" service="web" />
        </I18nextProvider>
      </QueryClientProvider>,
    )
  }

  it('sends tmpfs and ulimits with the revision, only for editable keys', async () => {
    fetchMock
      .mockResolvedValueOnce(json(resource))
      .mockResolvedValueOnce(json(resource))
    mount()
    await userEvent.type(
      await screen.findByLabelText('Tmpfs mounts'),
      '/run:size=16m',
    )
    await userEvent.type(screen.getByLabelText('Ulimits'), 'nofile=1024:4096')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    const [, init] = fetchMock.mock.calls[1] as [string, RequestInit]
    expect(init.method).toBe('PUT')
    expect((init.headers as Record<string, string>)['If-Match']).toBe(
      '"abc123"',
    )
    expect(JSON.parse(init.body as string)).toEqual({
      tmpfs: ['/run:size=16m'],
      ulimits: { nofile: { soft: 1024, hard: 4096 } },
      init: false,
    })
  })

  it('shows the server error at its field', async () => {
    fetchMock.mockResolvedValueOnce(json(resource)).mockResolvedValueOnce(
      json(
        {
          error: 'tmpfs: size required',
          code: 'invalid_runtime',
          errors: { tmpfs: 'size is required' },
        },
        422,
      ),
    )
    mount()
    await userEvent.type(await screen.findByLabelText('Tmpfs mounts'), '/run')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('size is required')).toBeTruthy()
  })
})
