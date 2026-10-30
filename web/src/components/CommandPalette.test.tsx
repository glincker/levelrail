import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import palette from '../locales/en/palette.json'
import common from '../locales/en/common.json'
import type { SearchHit } from '../queries/search'
import { appListQueryOptions } from '../queries/apps'
import { CommandPalette } from './CommandPalette'
import { createTestQueryClient } from '@/test/testQueryClient'

let experimentalOn: string[] = ['ai-chat']
const navigate = vi.fn()
const setTheme = vi.fn()
const restartApp = vi.fn()
const onOpenChange = vi.fn()
const router = { routesByPath: {} }

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  useRouter: () => router,
  useRouterState: ({
    select,
  }: {
    select: (s: { location: { pathname: string } }) => unknown
  }) => select({ location: { pathname: '/' } }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: (ns?: string) => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      const table: unknown = ns === 'palette' ? palette : common
      let cur: unknown = table
      for (const part of key.split('.')) {
        cur =
          cur && typeof cur === 'object'
            ? (cur as Record<string, unknown>)[part]
            : undefined
      }
      if (typeof cur !== 'string') {
        return typeof opts?.defaultValue === 'string' ? opts.defaultValue : key
      }
      return cur.replace(/\{\{(\w+)\}\}/g, (_, k: string) =>
        String((opts?.[k] as string | number | undefined) ?? ''),
      )
    },
  }),
}))
// jsdom has no layout, so the real virtualizer would window the list down to nothing.
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 32,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, i) => ({ index: i, start: i * 32 })),
    scrollToIndex: () => {},
  }),
}))
vi.mock('../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => experimentalOn,
}))
vi.mock('./ThemeProvider', () => ({
  useTheme: () => ({ theme: 'light', setTheme }),
}))
vi.mock('../hooks/usePaletteAppActions', () => ({
  usePaletteAppActions: () => ({ restartApp, redeployApp: vi.fn() }),
}))

const hit = (over: Partial<SearchHit> & Pick<SearchHit, 'kind' | 'title'>) => ({
  ref: `${over.kind}:${over.title}`,
  score: 1,
  ...over,
})

let searchHits: SearchHit[] = []
let templateRows: unknown[] = []
let searchStatus = 200
const fetchMock = vi.fn((url: string, init?: RequestInit) => {
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
  if (url.startsWith('/api/v1/search/saved')) {
    if (init?.method === 'POST') {
      return json({ id: 's1', name: 'x', query: 'y', created_at: '' })
    }
    return json({ saved: [{ id: 's9', name: 'prod apis', query: 'env:prod' }] })
  }
  if (url === '/api/v1/service-templates') return json(templateRows)
  if (url.startsWith('/api/v1/search/suggest')) return json({ suggestions: [] })
  if (url.startsWith('/api/v1/search/usage')) return json({}, 204)
  if (url.startsWith('/api/v1/search?')) {
    if (searchStatus !== 200) return json({ error: 'boom' }, searchStatus)
    return json({ hits: searchHits, total: searchHits.length })
  }
  return json({}, 404)
})

function renderPalette() {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <CommandPalette open onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  )
}

function searchCalls(): string[] {
  return fetchMock.mock.calls
    .map((c) => c[0])
    .filter((u) => u.startsWith('/api/v1/search?'))
}

beforeEach(() => {
  experimentalOn = ['ai-chat']
  searchHits = []
  templateRows = []
  searchStatus = 200
  window.localStorage.clear()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.clearAllMocks()
  vi.unstubAllGlobals()
})

describe('CommandPalette', () => {
  it('exposes combobox, listbox and option roles with hint chips', () => {
    renderPalette()
    expect(screen.getByRole('combobox')).toBeInTheDocument()
    expect(screen.getByRole('listbox')).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: /Go to Status/ }),
    ).toBeInTheDocument()
    expect(screen.getByText('Esc')).toBeInTheDocument()
  })

  it('fires no list-all or search requests just by opening', async () => {
    renderPalette()
    await screen.findByRole('option', { name: /prod apis/ })
    const urls = fetchMock.mock.calls.map((c) => c[0])
    // Mail actions need the mail status and the caller's abilities, both cheap reads.
    expect(
      urls.every(
        (u) =>
          u.startsWith('/api/v1/search') ||
          u.startsWith('/api/v1/mail/status') ||
          u.startsWith('/api/v1/users'),
      ),
    ).toBe(true)
    expect(searchCalls()).toHaveLength(0)
  })

  it('navigates from a static action by clicking', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('option', { name: /Go to Status/ }))
    expect(navigate).toHaveBeenCalledWith({ to: '/status', params: undefined })
  })

  it('opens the assistant from the first suggestion with Enter', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.keyboard('{Enter}')
    expect(navigate).toHaveBeenCalledWith({ to: '/ai-assistant' })
  })

  it('lists locked destinations while a beta feature is off', () => {
    experimentalOn = []
    renderPalette()
    expect(
      screen.getByRole('option', { name: /Load balancers/ }),
    ).toHaveTextContent(/Locked beta feature/)
  })

  it('searches the server once, debounced, and shows grouped hits', async () => {
    searchHits = [
      hit({ kind: 'app', title: 'api', ref: 'app:api' }),
      hit({
        kind: 'node',
        title: 'fsn1-a',
        ref: 'node:fsn1-a',
        href: '/nodes/1',
      }),
    ]
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'api')
    expect(
      await screen.findByRole('option', { name: /fsn1-a/ }),
    ).toBeInTheDocument()
    expect(searchCalls()).toHaveLength(1)
    expect(searchCalls()[0]).toContain('q=api')
    expect(screen.getByRole('option', { name: /^api/ })).toBeInTheDocument()
  })

  it('runs a confirm-gated action only after confirming', async () => {
    searchHits = [
      hit({
        kind: 'action',
        title: 'Restart api',
        ref: 'action:app.restart:api',
        app_name: 'api',
        href: '/apps/api?action=restart',
        action: { id: 'app.restart', ability: 'deploy', confirm: true },
      }),
    ]
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'restart api')
    await screen.findByRole('option', { name: /Restart api/ })
    await user.keyboard('{Enter}')
    expect(restartApp).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('Restart api?')
    await user.keyboard('{Enter}')
    expect(restartApp).toHaveBeenCalledWith('api')
    expect(
      fetchMock.mock.calls.some((c) => c[0] === '/api/v1/search/usage'),
    ).toBe(true)
  })

  it('cancels a pending confirm with Escape without closing', async () => {
    searchHits = [
      hit({
        kind: 'action',
        title: 'Stop api',
        ref: 'action:app.stop:api',
        href: '/apps/api?action=stop',
        action: { id: 'app.stop', ability: 'deploy', confirm: true },
      }),
    ]
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'stop')
    await screen.findByRole('option', { name: /Stop api/ })
    await user.keyboard('{Enter}')
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })

  it('turns a typed scope into a removable chip and sends it', async () => {
    const user = userEvent.setup()
    renderPalette()
    const input = screen.getByRole('combobox')
    await user.type(input, 'app:web restart')
    const chip = screen.getByRole('button', { name: 'Remove filter app:web' })
    await waitFor(() =>
      expect(searchCalls().some((u) => u.includes('q=app%3Aweb+restart'))).toBe(
        true,
      ),
    )
    await user.click(chip)
    expect(
      screen.queryByRole('button', { name: 'Remove filter app:web' }),
    ).toBeNull()
  })

  it('removes the last chip with Backspace on empty text', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'env:prod ')
    expect(
      screen.getByRole('button', { name: 'Remove filter env:prod' }),
    ).toBeInTheDocument()
    await user.keyboard('{Backspace}')
    expect(
      screen.queryByRole('button', { name: 'Remove filter env:prod' }),
    ).toBeNull()
  })

  it('suggests scope keys and kind values', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'ki')
    await user.click(await screen.findByRole('option', { name: 'kind:' }))
    await user.type(screen.getByRole('combobox'), 'act')
    await user.click(await screen.findByRole('option', { name: 'kind:action' }))
    expect(
      screen.getByRole('button', { name: 'Remove filter kind:action' }),
    ).toBeInTheDocument()
  })

  it('caps a group and expands it with Show all', async () => {
    searchHits = Array.from({ length: 7 }, (_, i) =>
      hit({ kind: 'domain', title: `site${i}.example.com`, href: '/d' }),
    )
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'site')
    await screen.findByRole('option', { name: /site0/ })
    expect(screen.queryByRole('option', { name: /site6/ })).toBeNull()
    await user.click(screen.getByRole('option', { name: 'Show all 7' }))
    expect(
      await screen.findByRole('option', { name: /site6/ }),
    ).toBeInTheDocument()
  })

  it('opens the hit in a new tab with Tab', async () => {
    const open = vi.fn()
    vi.stubGlobal('open', open)
    searchHits = [
      hit({ kind: 'database', title: 'main-db', href: '/databases/main-db' }),
    ]
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'main')
    await screen.findByRole('option', { name: /main-db/ })
    await user.keyboard('{Tab}')
    expect(open).toHaveBeenCalledWith(
      '/databases/main-db',
      '_blank',
      'noopener,noreferrer',
    )
  })

  it('navigates to a hit href with its query string split out', async () => {
    searchHits = [
      hit({
        kind: 'action',
        title: 'Deploy api',
        ref: 'action:app.deploy:api',
        href: '/apps/api?action=deploy',
        action: { id: 'app.deploy', ability: 'deploy', confirm: false },
      }),
    ]
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'deploy')
    await user.click(await screen.findByRole('option', { name: /Deploy api/ }))
    expect(navigate).toHaveBeenCalledWith({
      to: '/apps/api',
      search: { action: 'deploy' },
    })
  })

  it('clears the input on the first Escape and closes on the second', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'abc')
    await user.keyboard('{Escape}')
    expect(screen.getByRole('combobox')).toHaveValue('')
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    await user.keyboard('{Escape}')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows an error with Retry and the built-in commands as fallback', async () => {
    searchStatus = 500
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'status')
    expect(await screen.findByRole('button', { name: 'Retry' })).toBeVisible()
    expect(
      screen.getByRole('option', { name: /Go to Status/ }),
    ).toBeInTheDocument()
    searchStatus = 200
    searchHits = [hit({ kind: 'app', title: 'status-page', ref: 'app:s' })]
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByRole('option', { name: /status-page/ }),
    ).toBeInTheDocument()
  })

  it('suggests broadening when nothing matches', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'zzzzqq')
    expect(await screen.findByText(/Try fewer words/)).toBeInTheDocument()
  })

  it('applies a saved search from the idle list and saves the current one', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(await screen.findByRole('option', { name: /prod apis/ }))
    expect(
      screen.getByRole('button', { name: 'Remove filter env:prod' }),
    ).toBeInTheDocument()
    await user.click(await screen.findByRole('button', { name: 'Save search' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          (c) => c[0] === '/api/v1/search/saved' && c[1]?.method === 'POST',
        ),
      ).toBe(true),
    )
  })

  it('records a selection as a recent destination', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('option', { name: /Go to Nodes/ }))
    expect(window.localStorage.getItem('palette.recent')).toBe(
      JSON.stringify(['action-nodes']),
    )
  })

  it('ranks related pages for container and suggests a Portainer template', async () => {
    templateRows = [
      {
        id: 'portainer',
        name: 'Portainer',
        slogan: 'Manage Docker containers',
        category: 'tools',
        documentation_url: '',
        requires_configuration: false,
      },
    ]
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'container')
    await screen.findByRole('option', { name: /Portainer/ })
    const names = screen.getAllByRole('option').map((o) => o.textContent ?? '')
    const registry = names.findIndex((n) => /Registry credentials/.test(n))
    expect(registry).toBe(0)
    expect(names.findIndex((n) => /Portainer/.test(n))).toBeGreaterThan(
      registry,
    )
    expect(names.some((n) => /Go to Status/.test(n))).toBe(false)
  })

  it('puts Databases and Backups first when typing db', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'db')
    const names = (await screen.findAllByRole('option')).map(
      (o) => o.textContent ?? '',
    )
    expect(names[0]).toMatch(/Databases/)
    expect(names.slice(0, 3).join('|')).toMatch(/Backups/)
    expect(names.join('|')).toMatch(/Create database/)
  })

  it('shows a preview card for the highlighted item and follows ArrowDown', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'db')
    const preview = await screen.findByRole('complementary', {
      name: 'Preview',
    })
    expect(preview).toHaveTextContent('Managed databases')
    expect(preview).toHaveTextContent('/databases')
    await user.keyboard('{ArrowDown}')
    expect(preview).not.toHaveTextContent('Managed databases')
  })

  it('shows a Deploy action for a template preview', async () => {
    templateRows = [
      {
        id: 'portainer',
        name: 'Portainer',
        slogan: 'Manage Docker containers',
        category: 'tools',
        documentation_url: '',
        requires_configuration: false,
      },
    ]
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'portainer')
    await screen.findByRole('option', { name: /Portainer/ })
    const preview = screen.getByRole('complementary', { name: 'Preview' })
    expect(preview).toHaveTextContent('Template')
    await user.click(screen.getByRole('button', { name: /Deploy/ }))
    expect(navigate).toHaveBeenCalledWith({
      to: '/deploy',
      search: { template: 'portainer' },
    })
  })

  it('recommends create and import when there are no apps yet', () => {
    const client = createTestQueryClient()
    client.setQueryData(appListQueryOptions().queryKey, [])
    render(
      <QueryClientProvider client={client}>
        <CommandPalette open onOpenChange={onOpenChange} />
      </QueryClientProvider>,
    )
    expect(
      screen.getByRole('option', { name: /Bring existing apps/ }),
    ).toHaveTextContent(/Bring existing apps/)
  })

  it('drops the preview pane on narrow screens', () => {
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }))
    renderPalette()
    expect(screen.queryByRole('complementary')).toBeNull()
  })
})
