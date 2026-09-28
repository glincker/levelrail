import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AgentConnectCard } from './AgentConnectCard'
import { AgentTokensCard } from './AgentTokensCard'
import { CreateAgentTokenDialog } from './CreateAgentTokenDialog'
import { AGENT_PRESETS, buildMcpConfig } from '../../lib/agentConfig'
import type { TokenResource } from '../../types/token'

function withClient(node: ReactNode) {
  return (
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      {node}
    </QueryClientProvider>
  )
}

function token(id: string, extra: Partial<TokenResource> = {}): TokenResource {
  return {
    id,
    name: `tok-${id}`,
    abilities: ['read'],
    created_at: '2026-09-26T10:00:00Z',
    ...extra,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('buildMcpConfig', () => {
  it('references the token by env var and never embeds one', () => {
    const cfg = buildMcpConfig({
      serverKey: 'acme',
      binary: 'acme-mcp',
      mode: 'agent-core',
      apiURL: 'https://deploy.example.com',
    })
    const doc = JSON.parse(cfg) as {
      mcpServers: Record<
        string,
        { command: string; args: string[]; env: Record<string, string> }
      >
    }
    const server = doc.mcpServers.acme
    expect(server?.command).toBe('acme-mcp')
    expect(server?.args).toEqual(['--tool-profile', 'agent-core'])
    expect(server?.env.APP_API_TOKEN).toBe('${APP_API_TOKEN}')
    expect(server?.env.APP_API_URL).toBe(
      '${APP_API_URL:-https://deploy.example.com}',
    )
  })

  it.each(['read-only', 'standard', 'full'] as const)(
    'uses --mode %s',
    (mode) => {
      const cfg = buildMcpConfig({
        serverKey: 'a',
        binary: 'b',
        mode,
        apiURL: '',
      })
      expect(cfg).toContain(`"--mode",\n        "${mode}"`)
      expect(cfg).not.toContain('APP_API_URL')
    },
  )
})

describe('AGENT_PRESETS', () => {
  it('never grants root', () => {
    for (const p of AGENT_PRESETS) {
      expect(p.abilities).not.toContain('root')
    }
    expect(AGENT_PRESETS.map((p) => p.abilities)).toEqual([
      ['read'],
      ['read', 'deploy'],
      ['read', 'read:sensitive', 'write', 'write:sensitive', 'deploy'],
    ])
  })
})

describe('AgentConnectCard', () => {
  it('shows the snippet and the mode table, and switches mode', async () => {
    render(
      <AgentConnectCard
        serverKey="acme"
        binary="acme-mcp"
        apiURL="https://deploy.example.com"
      />,
    )
    const snippet = screen.getByLabelText('MCP config')
    expect(snippet).toHaveTextContent('--tool-profile')
    expect(screen.getByText('41,600')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('combobox', { name: 'Tool mode' }))
    await userEvent.click(await screen.findByRole('option', { name: 'Full' }))
    await waitFor(() => {
      expect(snippet).toHaveTextContent('"--mode"')
    })
    expect(snippet).toHaveTextContent('"full"')
  })
})

describe('AgentTokensCard', () => {
  it('lists only tokens with an agent label, with last used', () => {
    render(
      withClient(
        <AgentTokensCard
          tokens={[
            token('1', {
              agent: { name: 'Claude Code' },
              last_used_at: '2026-09-26T09:00:00Z',
            }),
            token('2'),
            token('3', {
              agent: { name: 'Old bot' },
              revoked_at: '2026-09-01T00:00:00Z',
            }),
          ]}
        />,
      ),
    )
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.getByText('Old bot')).toBeInTheDocument()
    expect(screen.queryByText('tok-2')).not.toBeInTheDocument()
    expect(screen.getByText('Revoked')).toBeInTheDocument()
    const row = screen.getByText('Claude Code').closest('tr')
    if (!row) throw new Error('row not found')
    expect(within(row).getByRole('time')).toBeInTheDocument()
  })

  it('shows an empty state without agent tokens', () => {
    render(withClient(<AgentTokensCard tokens={[token('2')]} />))
    expect(screen.getByText('No agent tokens yet')).toBeInTheDocument()
  })
})

describe('CreateAgentTokenDialog', () => {
  it('creates a token with the chosen preset and shows the secret once', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >(() =>
      Promise.resolve({
        ok: true,
        status: 201,
        json: () =>
          Promise.resolve({
            ...token('9', { agent: { name: 'Claude Code' } }),
            token: 'secret-xyz',
          }),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(withClient(<CreateAgentTokenDialog />))

    await userEvent.click(
      screen.getByRole('button', { name: 'Create agent token' }),
    )
    const submit = screen.getByRole('button', { name: 'Create token' })
    expect(submit).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Agent name'), 'Claude Code')
    await userEvent.click(screen.getByRole('radio', { name: /Deployer/ }))
    await userEvent.click(submit)

    expect(await screen.findByText('secret-xyz')).toBeInTheDocument()
    const raw = fetchMock.mock.calls[0]?.[1]?.body
    if (typeof raw !== 'string') throw new Error('expected a JSON body')
    expect(JSON.parse(raw)).toEqual({
      name: 'Claude Code',
      abilities: ['read', 'deploy'],
      agent: { name: 'Claude Code' },
    })
  })
})
