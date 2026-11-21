import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../locales/en/nodes.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { InstallAgentBanner } from './InstallAgentBanner'
import { looksLikeMac } from '../lib/installAgent'

vi.mock('../lib/pollTier', () => ({ pollMs: () => 50 }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

const MAC_UA = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
const LINUX_UA = 'Mozilla/5.0 (X11; Linux x86_64)'

interface Api {
  mints: number
  nodes: { id: string; name: string }[]
}

function mockApi(api: Api, macos?: string) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const json = (body: unknown, status = 200) =>
        Promise.resolve(new Response(JSON.stringify(body), { status }))
      if (url === '/api/v1/nodes/join-tokens' && init?.method === 'POST') {
        api.mints++
        return json(
          {
            token: 'tok',
            expires_at: '2030-01-01T00:00:00Z',
            join_commands: {
              docker: 'docker run agent',
              binary: 'sudo agent',
              ...(macos ? { macos } : {}),
            },
          },
          201,
        )
      }
      if (url === '/api/v1/nodes') return json(api.nodes)
      return json({ configured: false, agent_addr: 'cp.example:9443' })
    }),
  )
}

function renderBanner() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>
        <InstallAgentBanner />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  window.localStorage.clear()
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('looksLikeMac', () => {
  it('reads the user agent', () => {
    expect(looksLikeMac(MAC_UA)).toBe(true)
    expect(looksLikeMac(LINUX_UA)).toBe(false)
  })
})

describe('InstallAgentBanner', () => {
  it('stays hidden when more than one node is connected', async () => {
    mockApi({
      mints: 0,
      nodes: [
        { id: 'a', name: 'a' },
        { id: 'b', name: 'b' },
      ],
    })
    renderBanner()
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(
      screen.queryByText('Run builds and apps on another machine'),
    ).toBeNull()
  })

  it('mints a token only when opened and opens the Mac tab on a Mac', async () => {
    vi.stubGlobal('navigator', { ...navigator, userAgent: MAC_UA })
    const api: Api = { mints: 0, nodes: [{ id: 'a', name: 'a' }] }
    mockApi(api, 'brew install x')
    renderBanner()
    const open = await screen.findByRole('button', { name: 'Add a server' })
    expect(api.mints).toBe(0)
    await userEvent.click(open)
    expect(await screen.findByRole('tab', { name: 'macOS' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(api.mints).toBe(1)
  })

  it('opens the Linux tab elsewhere and hides macOS when the API omits it', async () => {
    vi.stubGlobal('navigator', { ...navigator, userAgent: MAC_UA })
    mockApi({ mints: 0, nodes: [] })
    renderBanner()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Add a server' }),
    )
    expect(await screen.findByText('docker run agent')).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'macOS' })).toBeNull()
    expect(screen.getByRole('tab', { name: 'Linux' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
  })

  it('flips from waiting to connected when a new node appears', async () => {
    const api: Api = { mints: 0, nodes: [{ id: 'a', name: 'a' }] }
    mockApi(api)
    renderBanner()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Add a server' }),
    )
    expect(
      await screen.findByText('Waiting for the node to connect'),
    ).toBeInTheDocument()
    api.nodes = [...api.nodes, { id: 'b', name: 'laptop' }]
    await waitFor(
      () => expect(screen.getByText('Connected: laptop')).toBeInTheDocument(),
      { timeout: 3000 },
    )
  })

  it('stays hidden once dismissed', async () => {
    mockApi({ mints: 0, nodes: [] })
    renderBanner()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Hide this banner' }),
    )
    expect(
      screen.queryByText('Run builds and apps on another machine'),
    ).toBeNull()
  })
})
