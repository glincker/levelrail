import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AgentFilterChips } from './AgentFilterChips'
import { AUDIT_VIRTUALIZE_OVER, AuditLogTable } from './AuditLogTable'
import { CreateTokenDialog } from './CreateTokenDialog'
import { TokenTable } from './TokenTable'
import { collectAgentNames } from '../lib/agentNames'
import { buildAuditLogParams, type AuditLogEntry } from '../queries/auditLog'
import settingsEn from '../locales/en/settings.json'
import type { TokenResource } from '../types/token'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function entry(i: number, agent?: string): AuditLogEntry {
  return {
    id: `e${i}`,
    actor_type: 'token',
    actor_id: 't1',
    actor_name: 'ci',
    ability: 'deploy',
    method: 'POST',
    path: `/api/v1/apps/a${i}/deploy`,
    status_code: 200,
    remote_addr: '127.0.0.1',
    created_at: '2026-09-26T10:00:00Z',
    client_kind: 'mcp',
    agent_name: agent,
  }
}

function token(id: string, agent?: string): TokenResource {
  return {
    id,
    name: `tok-${id}`,
    abilities: ['read'],
    created_at: '2026-09-26T10:00:00Z',
    ...(agent ? { agent: { name: agent, description: 'desc' } } : {}),
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('collectAgentNames', () => {
  it('merges token labels and entries, sorted, keeping the active filter', () => {
    const names = collectAgentNames(
      [token('1', 'Zed'), token('2')],
      [entry(1, 'Claude Code'), entry(2)],
      'Ghost',
    )
    expect(names).toEqual(['Claude Code', 'Ghost', 'Zed'])
  })
})

describe('buildAuditLogParams', () => {
  it('sends the agent filter', () => {
    expect(buildAuditLogParams({ agent: 'Claude Code' }).get('agent')).toBe(
      'Claude Code',
    )
    expect(buildAuditLogParams({}).has('agent')).toBe(false)
  })
})

describe('AgentFilterChips', () => {
  it('toggles a chip on and off', async () => {
    const onChange = vi.fn<(agent: string | undefined) => void>()
    const { rerender } = render(
      <AgentFilterChips
        agents={['A', 'B']}
        active={undefined}
        onChange={onChange}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: 'B' }))
    expect(onChange).toHaveBeenLastCalledWith('B')

    rerender(
      <AgentFilterChips agents={['A', 'B']} active="B" onChange={onChange} />,
    )
    expect(screen.getByRole('button', { name: 'B' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    await userEvent.click(screen.getByRole('button', { name: 'B' }))
    expect(onChange).toHaveBeenLastCalledWith(undefined)
  })

  it('renders nothing without agents', () => {
    const { container } = render(
      <AgentFilterChips agents={[]} active={undefined} onChange={vi.fn()} />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe('AuditLogTable', () => {
  it('shows the agent name or None in the Agent column', () => {
    render(<AuditLogTable entries={[entry(1, 'Claude Code'), entry(2)]} />)
    expect(
      screen.getByRole('columnheader', { name: 'Agent' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.getByText('None')).toBeInTheDocument()
  })

  it('switches to the virtualized grid past the threshold', () => {
    const many = Array.from({ length: AUDIT_VIRTUALIZE_OVER + 1 }, (_, i) =>
      entry(i),
    )
    render(<AuditLogTable entries={many} />)
    expect(screen.getByRole('table', { name: 'Audit log' })).toHaveAttribute(
      'aria-rowcount',
      String(many.length + 1),
    )
    expect(
      screen.getByRole('columnheader', { name: 'Agent' }),
    ).toBeInTheDocument()
  })
})

describe('TokenTable', () => {
  it('shows the agent label', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <TokenTable tokens={[token('1', 'Claude Code'), token('2')]} />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    expect(
      screen.getByRole('columnheader', { name: 'Agent' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
  })
})

describe('CreateTokenDialog agent name', () => {
  it('posts the agent label with the token', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >(() =>
      Promise.resolve({
        ok: true,
        status: 201,
        json: () =>
          Promise.resolve({ ...token('9', 'Claude Code'), token: 'secret-1' }),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchMock)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <CreateTokenDialog />
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }))
    await userEvent.type(screen.getByLabelText('Name'), 'agent-tok')
    await userEvent.type(screen.getByLabelText(/Agent name/), 'Claude Code')
    const readBox = screen.getAllByRole('checkbox')[0]
    if (!readBox) throw new Error('no ability checkbox')
    await userEvent.click(readBox)
    await userEvent.click(
      screen.getAllByRole('button', { name: 'Create token' }).at(-1)!,
    )

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    const rawBody = fetchMock.mock.calls[0]?.[1]?.body
    if (typeof rawBody !== 'string') throw new Error('expected a JSON body')
    const body = JSON.parse(rawBody) as { agent?: { name: string } }
    expect(body.agent).toEqual({ name: 'Claude Code' })
    expect(await screen.findByText('secret-1')).toBeInTheDocument()
  })
})
