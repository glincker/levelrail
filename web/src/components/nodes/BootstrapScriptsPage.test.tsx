import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import scriptsEn from '../../locales/en/nodes.json'
import { BootstrapScriptsPage } from './BootstrapScriptsPage'

const root = vi.hoisted(() => ({ value: true }))
vi.mock('@/hooks/useIsRoot', () => ({ useIsRoot: () => root.value }))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({
    children,
    actions,
  }: {
    children?: ReactNode
    actions?: ReactNode
  }) => (
    <div>
      {actions}
      {children}
    </div>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'nodes'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, nodes: scriptsEn } },
  interpolation: { escapeValue: false },
})

const posts: unknown[] = []

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <BootstrapScriptsPage />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('BootstrapScriptsPage', () => {
  beforeEach(() => {
    root.value = true
    posts.length = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        await Promise.resolve()
        if (init?.method === 'POST') {
          posts.push(JSON.parse(init.body as string))
          return Response.json({}, { status: 201 })
        }
        if (url.endsWith('/limits')) {
          return Response.json({ max_bytes: 1024, max_count: 10 })
        }
        return Response.json([
          {
            id: 's1',
            name: 'install-agent',
            size_bytes: 120,
            created_at: '2026-11-01T00:00:00Z',
            updated_at: '2026-11-02T00:00:00Z',
          },
        ])
      }),
    )
  })
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('creates a script as root', async () => {
    const user = userEvent.setup()
    renderPage()
    expect(await screen.findByText('install-agent')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'New script' }))
    await user.type(screen.getByLabelText('Name'), 'harden')
    await user.type(screen.getByLabelText('Script'), 'echo hi')
    await user.click(screen.getByRole('button', { name: 'Save script' }))
    await waitFor(() =>
      expect(posts).toEqual([{ name: 'harden', body: 'echo hi' }]),
    )
  })

  it('hides every mutation for a non-root account', async () => {
    root.value = false
    renderPage()
    expect(await screen.findByText('install-agent')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'New script' })).toBeNull()
    expect(screen.queryByRole('button', { name: /^Delete / })).toBeNull()
    expect(screen.getByText(/Only a root account/)).toBeTruthy()
  })
})
