import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ModelRow } from './ModelRow'
import type { ModelResource } from '../types/models'

function model(overrides: Partial<ModelResource> = {}): ModelResource {
  return {
    name: 'chat',
    engine: 'ollama',
    model: 'llama3.1:8b',
    node_id: '',
    gpu_count: -1,
    endpoint_url: 'https://chat.example.com/v1',
    api_key_prefix: 'lr-abcd',
    hf_token_set: false,
    status: {
      ready: false,
      reason: 'Downloading',
      message: 'downloading: 12%',
    },
    created_at: '2026-09-24T00:00:00Z',
    updated_at: '2026-09-24T00:00:00Z',
    ...overrides,
  }
}

function renderRow(m: ModelResource) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ModelRow model={m} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ModelRow', () => {
  it('shows progress detail while downloading', () => {
    renderRow(model())
    expect(screen.getByText('chat')).toBeInTheDocument()
    expect(screen.getByText('Downloading')).toBeInTheDocument()
    expect(screen.getByText('downloading: 12%')).toBeInTheDocument()
    expect(screen.getByText('https://chat.example.com/v1')).toBeInTheDocument()
  })

  it('shows Loaded when ready', () => {
    renderRow(
      model({ status: { ready: true, reason: 'ModelLoaded', message: 'ok' } }),
    )
    expect(screen.getByText('Loaded')).toBeInTheDocument()
  })

  it('shows a blocked reason as an error state', () => {
    renderRow(
      model({
        status: {
          ready: false,
          reason: 'GPURuntimeMissing',
          message: 'Install nvidia-container-toolkit',
        },
      }),
    )
    expect(screen.getByText('GPURuntimeMissing')).toBeInTheDocument()
  })

  it('disables actions while the model is being deleted', () => {
    renderRow(
      model({ status: { ready: false, reason: 'Deleting', message: '' } }),
    )
    expect(screen.getByRole('button', { name: 'Restart chat' })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Rotate API key of chat' }),
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Delete chat' })).toBeDisabled()
  })

  it('reveals the rotated key once', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ api_key: 'lr-brandnewkey' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    renderRow(model())
    await userEvent.click(
      screen.getByRole('button', { name: 'Rotate API key of chat' }),
    )
    await waitFor(() => {
      expect(screen.getByTestId('model-api-key')).toHaveTextContent(
        'lr-brandnewkey',
      )
    })
    expect(fetch).toHaveBeenCalledWith('/api/v1/models/chat/api-key', {
      method: 'POST',
    })
    await userEvent.click(
      screen.getByRole('button', { name: 'I have saved the key' }),
    )
    await waitFor(() => {
      expect(screen.queryByTestId('model-api-key')).not.toBeInTheDocument()
    })
  })
})
