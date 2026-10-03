import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CommandPalette } from './CommandPalette'

let experimentalOn: string[] = ['ai-chat']
const navigate = vi.fn()
const setTheme = vi.fn()
const restartApp = vi.fn()
const redeployApp = vi.fn()
const listApps = vi.fn(() =>
  Promise.resolve([
    { name: 'web', image: 'nginx:1.27', status: { variant: 'success' } },
  ]),
)

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  useRouterState: ({
    select,
  }: {
    select: (s: { location: { pathname: string } }) => unknown
  }) => select({ location: { pathname: '/' } }),
}))
vi.mock('../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => experimentalOn,
}))
vi.mock('./ThemeProvider', () => ({
  useTheme: () => ({ theme: 'light', setTheme }),
}))
vi.mock('../hooks/usePaletteAppActions', () => ({
  usePaletteAppActions: () => ({ restartApp, redeployApp }),
}))
vi.mock('../queries/apps', () => ({
  appListQueryOptions: () => ({ queryKey: ['apps'], queryFn: listApps }),
}))
vi.mock('../queries/databases', () => ({
  databaseListQueryOptions: () => ({
    queryKey: ['databases'],
    queryFn: () => Promise.resolve([]),
  }),
}))
vi.mock('../queries/nodes', () => ({
  nodeListQueryOptions: () => ({
    queryKey: ['nodes'],
    queryFn: () =>
      Promise.resolve([{ id: 'node-1', name: 'fsn1-a', status: 'ready' }]),
  }),
}))
vi.mock('../queries/serviceTemplates', () => ({
  serviceTemplatesQueryOptions: () => ({
    queryKey: ['service-templates'],
    queryFn: () =>
      Promise.resolve([
        { id: 'postgres', name: 'PostgreSQL', category: 'database' },
      ]),
  }),
}))
vi.mock('../queries/domains', () => ({
  domainsQueryOptions: () => ({
    queryKey: ['domains'],
    queryFn: () =>
      Promise.resolve([{ domain: 'app.example.com', service_name: 'web' }]),
  }),
}))

function renderPalette() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <CommandPalette open onOpenChange={() => {}} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  experimentalOn = ['ai-chat']
  window.localStorage.clear()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('CommandPalette', () => {
  it('exposes combobox, listbox and option roles with hint chips', () => {
    renderPalette()
    expect(screen.getByRole('combobox')).toBeInTheDocument()
    expect(screen.getByRole('listbox')).toBeInTheDocument()
    expect(screen.getByRole('group', { name: 'Actions' })).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: /Go to Status/ }),
    ).toBeInTheDocument()
    expect(screen.getByText('Esc')).toBeInTheDocument()
  })

  it('opens the assistant from the first suggestion with Enter', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.keyboard('{Enter}')
    expect(navigate).toHaveBeenCalledWith({
      to: '/ai-assistant',
      params: undefined,
    })
  })

  it('navigates from the Actions group by clicking', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('option', { name: /Go to Status/ }))
    expect(navigate).toHaveBeenCalledWith({ to: '/status', params: undefined })
  })

  it('toggles the theme', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('option', { name: /Toggle theme/ }))
    expect(setTheme).toHaveBeenCalledWith('dark')
  })

  it('offers per-app quick actions and runs restart and redeploy', async () => {
    const user = userEvent.setup()
    renderPalette()
    await waitFor(() => expect(listApps).toHaveBeenCalled())
    await screen.findByRole('option', { name: 'web' })
    await user.type(screen.getByRole('combobox'), 'web')

    expect(
      await screen.findByRole('option', { name: 'Redeploy web' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Open logs for web' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Open deploys for web' }),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: 'Restart web' }))
    expect(restartApp).toHaveBeenCalledWith('web')
  })

  it('redeploys with the app image and opens logs', async () => {
    const user = userEvent.setup()
    renderPalette()
    await screen.findByRole('option', { name: 'web' })
    await user.type(screen.getByRole('combobox'), 'web')
    await user.click(
      await screen.findByRole('option', { name: 'Redeploy web' }),
    )
    expect(redeployApp).toHaveBeenCalledWith('web', 'nginx:1.27')
  })

  it('opens logs for the app', async () => {
    const user = userEvent.setup()
    renderPalette()
    await screen.findByRole('option', { name: 'web' })
    await user.type(screen.getByRole('combobox'), 'web')
    await user.click(
      await screen.findByRole('option', { name: 'Open logs for web' }),
    )
    expect(navigate).toHaveBeenCalledWith({
      to: '/apps/$name/logs',
      params: { name: 'web' },
    })
  })

  it('finds and navigates to a node by name', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'fsn1-a')
    await user.click(await screen.findByRole('option', { name: 'fsn1-a' }))
    expect(navigate).toHaveBeenCalledWith({
      to: '/nodes/$id',
      params: { id: 'node-1' },
    })
  })

  it('finds and navigates to a template by name', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'PostgreSQL')
    await user.click(await screen.findByRole('option', { name: 'PostgreSQL' }))
    expect(navigate).toHaveBeenCalledWith({
      to: '/templates/$id',
      params: { id: 'postgres' },
    })
  })

  it('finds and navigates to a domain by name', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'app.example.com')
    await user.click(
      await screen.findByRole('option', { name: 'app.example.com' }),
    )
    expect(navigate).toHaveBeenCalledWith({
      to: '/apps/$name/domains',
      params: { name: 'web' },
    })
  })

  it('does not refetch the app list per keystroke', async () => {
    const user = userEvent.setup()
    renderPalette()
    await screen.findByRole('option', { name: 'web' })
    await user.type(screen.getByRole('combobox'), 'restart')
    expect(listApps).toHaveBeenCalledTimes(1)
  })

  it('shows a Recent group from localStorage', () => {
    window.localStorage.setItem(
      'palette.recent',
      JSON.stringify(['action-nodes', 'gone-key']),
    )
    renderPalette()
    const recent = screen.getByRole('group', { name: 'Recent' })
    expect(recent).toHaveTextContent('Go to Nodes')
    expect(recent).not.toHaveTextContent('gone-key')
  })

  it('records a selection as recent', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('option', { name: /Go to Nodes/ }))
    expect(window.localStorage.getItem('palette.recent')).toBe(
      JSON.stringify(['action-nodes']),
    )
  })

  it('shows an empty state when nothing matches', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.type(screen.getByRole('combobox'), 'zzzzqq')
    expect(screen.getByText('No results.')).toBeInTheDocument()
  })

  it('hides gated destinations and the assistant suggestion while off', () => {
    experimentalOn = []
    renderPalette()
    expect(screen.queryByRole('option', { name: /Load balancers/ })).toBeNull()
    expect(screen.queryByRole('option', { name: /AI models/ })).toBeNull()
    expect(screen.queryByRole('option', { name: /AI assistant/ })).toBeNull()
    expect(
      screen.queryByRole('option', { name: /Ask the assistant/ }),
    ).toBeNull()
    expect(
      screen.getByRole('option', { name: /Databases/ }),
    ).toBeInTheDocument()
  })

  it('shows static quick-action CTAs with no recents and no app data', () => {
    experimentalOn = []
    renderPalette()
    const suggested = screen.getByRole('group', { name: 'Suggested' })
    expect(suggested).toHaveTextContent('Create app')
    expect(suggested).toHaveTextContent('Go to Apps')
    expect(suggested).toHaveTextContent('Deployments')
    expect(suggested).toHaveTextContent('Help')
  })

  it('shows gated destinations once their feature is on', () => {
    experimentalOn = ['ai-chat', 'ai-models', 'load-balancer']
    renderPalette()
    expect(
      screen.getByRole('option', { name: /Load balancers/ }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: /AI models/ }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: /AI assistant/ }),
    ).toBeInTheDocument()
  })
})
