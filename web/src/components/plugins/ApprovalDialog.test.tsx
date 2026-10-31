import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/plugins.json'
import type { Plugin } from '../../queries/plugins'
import { ApprovalDialog } from './ApprovalDialog'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['plugins'],
  defaultNS: 'plugins',
  resources: { en: { plugins: en } },
  interpolation: { escapeValue: false },
})

function plugin(over: Partial<Plugin> = {}): Plugin {
  return {
    name: 'example-notify',
    version: '0.1.0',
    kind: 'notification_channel',
    publisher: 'Example Org',
    description: '',
    image: 'registry.example.com/x@sha256:' + '0'.repeat(64),
    digest: 'sha256:' + '0'.repeat(64),
    manifest: '',
    key_id: 'ed25519:abc',
    requested_abilities: ['read', 'read:sensitive'],
    approved_abilities: [],
    outbound_hosts: [],
    config_json: '{}',
    state: 'installed',
    running: false,
    health: 'stopped',
    verification: 'verified',
    fix: 'approve',
    panels: [],
    approved_at: '',
    approved_by: '',
    installed_at: '',
    installed_by: '',
    last_error: '',
    ...over,
  }
}

function renderDialog(p: Plugin, onClose = vi.fn()) {
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <ApprovalDialog plugin={p} onClose={onClose} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return onClose
}

afterEach(() => vi.unstubAllGlobals())

describe('ApprovalDialog', () => {
  it('shows what is requested and approves nothing by default', () => {
    renderDialog(plugin({ outbound_hosts: ['hooks.example.com'] }))
    expect(screen.getByText(/hooks\.example\.com/)).toBeTruthy()
    expect(screen.getByText(/general network access/)).toBeTruthy()
    const approve = screen.getByRole('button', { name: 'Approve selected' })
    expect((approve as HTMLButtonElement).disabled).toBe(true)
    for (const box of screen.getAllByRole('checkbox')) {
      expect(box.getAttribute('aria-checked')).toBe('false')
    }
  })

  it('sends only the ticked abilities', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(plugin()), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    const onClose = renderDialog(plugin())
    await user.click(screen.getAllByRole('checkbox').at(0) as HTMLElement)
    await user.click(screen.getByRole('button', { name: 'Approve selected' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/plugins/example-notify/approve')
    expect(JSON.parse(init.body as string)).toEqual({ abilities: ['read'] })
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })
})
