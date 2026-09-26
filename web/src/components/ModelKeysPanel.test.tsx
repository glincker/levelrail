import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ModelKeysPanel } from './ModelKeysPanel'
import type { ModelKey } from '../types/models'

const KEY: ModelKey = {
  id: 'key-1',
  name: 'ci',
  key_prefix: 'lr-abcd12',
  status: 'active',
  rpm: 0,
  tpm: 0,
  tpd: 5000,
  max_parallel: 0,
  allow_paths: [],
  allow_models: ['llama'],
  in_flight: 0,
  created_at: '2026-09-24T00:00:00Z',
  created_by: 'Ada',
}

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ModelKeysPanel modelName="chat" baseUrl="https://chat.example.com/v1" />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ModelKeysPanel', () => {
  it('lists keys and asks before revoking', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify([KEY]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )
    renderPanel()
    expect(await screen.findByText('ci')).toBeInTheDocument()
    expect(screen.getByText('5000 tok/day')).toBeInTheDocument()
    expect(screen.getByText('by Ada')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Revoke key ci'))
    expect(
      await screen.findByRole('button', { name: 'Revoke key' }),
    ).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
