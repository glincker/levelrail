import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ModelResource } from '../../types/models'
import { ModelResidencyCard } from './ModelResidencyCard'

function model(over: Partial<ModelResource>): ModelResource {
  return {
    name: 'chat',
    engine: 'ollama',
    model: 'llama3.1:8b',
    node_id: '',
    gpu_count: -1,
    api_key_prefix: 'lr-abcd',
    hf_token_set: false,
    status: { ready: false, reason: 'Idle' },
    created_at: '2026-09-24T00:00:00Z',
    updated_at: '2026-09-24T00:00:00Z',
    residency: 'on_demand',
    idle_ttl_seconds: 600,
    effective_idle_ttl_seconds: 600,
    residency_state: 'asleep',
    ...over,
  }
}

function renderCard(m: ModelResource) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ModelResidencyCard model={m} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ModelResidencyCard', () => {
  it('shows an idle on-demand model and lets you wake it', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 204 }))
    renderCard(model({}))
    expect(screen.getByText('Idle, engine stopped')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /sleep now/i })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: /wake now/i }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/models/chat/wake',
        expect.objectContaining({ method: 'POST' }),
      )
    })
  })

  it('saves the mode and idle time in seconds', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 204 }))
    renderCard(model({ residency_state: 'awake' }))
    const input = screen.getByLabelText('Idle minutes')
    await userEvent.clear(input)
    await userEvent.type(input, '30')
    await userEvent.click(screen.getAllByRole('button', { name: 'Save' })[0]!)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/models/chat/residency')
    expect(JSON.parse(init.body as string)).toEqual({
      residency: 'on_demand',
      idle_ttl_seconds: 1800,
    })
  })

  it('saves a swap group', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 204 }))
    renderCard(model({ residency: 'always', residency_state: 'awake' }))
    const input = screen.getByLabelText('Group name')
    await userEvent.type(input, 'gpu0')
    await userEvent.click(screen.getAllByRole('button', { name: 'Save' })[1]!)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/models/chat/swap-group')
    expect(JSON.parse(init.body as string)).toEqual({ swap_group: 'gpu0' })
  })

  it('shows which models it shares a GPU with', () => {
    renderCard(
      model({
        residency: 'always',
        residency_state: 'awake',
        swap_group: 'gpu0',
        shares_gpu_with: ['other-model'],
      }),
    )
    expect(screen.getByText('Shares a GPU with:')).toBeInTheDocument()
    expect(screen.getByText('other-model')).toBeInTheDocument()
  })

  it('has no wake, sleep or idle minutes for an always-loaded model', () => {
    renderCard(
      model({
        residency: 'always',
        residency_state: 'awake',
        idle_ttl_seconds: 0,
      }),
    )
    expect(screen.getAllByText('Always loaded').length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: /wake now/i })).toBeNull()
    expect(screen.queryByLabelText('Idle minutes')).toBeNull()
  })
})
