import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ModelPreflightPanel } from './ModelPreflightPanel'
import type { PreflightResult } from '../types/modelPreflight'

function result(over: Partial<PreflightResult> = {}): PreflightResult {
  return {
    repo: 'acme/chat-GGUF',
    status: 'ok',
    exists: true,
    message: 'Repository found.',
    private: false,
    license: 'mit',
    total_bytes: 9 * 1024 ** 3,
    file_count: 4,
    files: [],
    files_truncated: false,
    has_gguf: true,
    has_safetensors: false,
    compatible_engines: ['llamacpp', 'ollama'],
    engine_hint: 'GGUF only: use llama.cpp (or Ollama).',
    quants: [
      {
        name: 'Q4_K_M',
        bytes: 2 * 1024 ** 3,
        files: [],
        fit: 'fits',
        recommended: true,
      },
      {
        name: 'Q8_0',
        bytes: 4 * 1024 ** 3,
        files: [],
        fit: 'wont_fit',
        recommended: false,
      },
    ],
    node: {
      node_id: '',
      gpu_present: true,
      vram_total_bytes: null,
      vram_free_bytes: null,
      disk_free_bytes: null,
      disk_total_bytes: null,
    },
    disk: {
      status: 'unknown',
      required_bytes: 0,
      free_bytes: null,
      message: 'This node has not reported its disk yet.',
    },
    warnings: [],
    estimate_note: 'Fit figures are estimates.',
    cached: false,
    ...over,
  }
}

function mockHub(body: PreflightResult | { error: string }, status = 200) {
  const fetchMock = vi.fn<
    (url: string, init?: RequestInit) => Promise<Response>
  >(() => Promise.resolve(new Response(JSON.stringify(body), { status })))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function sentBody(init: RequestInit | undefined): string {
  return typeof init?.body === 'string' ? init.body : ''
}

function renderPanel(
  props: Partial<Parameters<typeof ModelPreflightPanel>[0]> = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const onPickModel = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <ModelPreflightPanel
        engine="llamacpp"
        model="acme/chat-GGUF"
        node="local"
        hfToken=""
        onPickModel={onPickModel}
        {...props}
      />
    </QueryClientProvider>,
  )
  return { onPickModel }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ModelPreflightPanel', () => {
  it('renders nothing for a model field that is not a Hub repo', () => {
    const fetchMock = mockHub(result())
    renderPanel({ model: 'llama3.1:8b' })
    expect(
      screen.queryByRole('region', { name: 'Hugging Face check' }),
    ).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not render or fetch for Ollama tags', () => {
    const fetchMock = mockHub(result())
    renderPanel({ engine: 'ollama', model: 'llama3.1:8b' })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('shows size, license, quants and fit pills only where the fit is known, and picks a quant', async () => {
    const fetchMock = mockHub(
      result({
        quants: [
          {
            name: 'Q2_K',
            bytes: 1024 ** 3,
            files: [],
            fit: 'unknown',
            recommended: false,
          },
          {
            name: 'Q4_K_M',
            bytes: 2 * 1024 ** 3,
            files: [],
            fit: 'fits',
            recommended: true,
          },
          {
            name: 'Q8_0',
            bytes: 4 * 1024 ** 3,
            files: [],
            fit: 'wont_fit',
            recommended: false,
          },
        ],
      }),
    )
    const { onPickModel } = renderPanel()

    expect(await screen.findByText('Found')).toBeInTheDocument()
    expect(screen.getByText('License: mit')).toBeInTheDocument()
    expect(screen.getByText('9.0 GiB in 4 files')).toBeInTheDocument()
    expect(screen.getByText('Fits')).toBeInTheDocument()
    expect(screen.getByText("Won't fit")).toBeInTheDocument()
    expect(screen.queryByText('Unknown')).toBeNull()
    expect(screen.getByText('recommended')).toBeInTheDocument()
    expect(screen.getByText(/Fit figures are estimates/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Q4_K_M/ }))
    expect(onPickModel).toHaveBeenCalledWith('acme/chat-GGUF:Q4_K_M')

    const body = JSON.parse(sentBody(fetchMock.mock.calls[0]?.[1])) as Record<
      string,
      unknown
    >
    expect(body).toMatchObject({ repo: 'acme/chat-GGUF', engine: 'llamacpp' })
    expect(body).not.toHaveProperty('hf_token')
  })

  it('explains a gated repo and its next step', async () => {
    mockHub(
      result({
        status: 'gated',
        gated: 'manual',
        message:
          'This repository is gated: Hugging Face requires an accepted license and an access token.',
        next_step:
          'Accept the license on huggingface.co, then enter a Hugging Face access token in the Hugging Face token field.',
        quants: [],
      }),
    )
    renderPanel({ model: 'acme/gated' })
    expect(await screen.findByText('Gated')).toBeInTheDocument()
    expect(screen.getByText(/This repository is gated/)).toBeInTheDocument()
    expect(screen.getByText(/Hugging Face token field/)).toBeInTheDocument()
  })

  it('shows a not found state without size or quant details', async () => {
    mockHub(
      result({
        status: 'not_found',
        exists: false,
        message: 'no such repository, or it is private',
        next_step: 'Check the spelling (owner/name).',
        quants: [],
      }),
    )
    renderPanel({ model: 'acme/none' })
    expect(await screen.findByText('Not found')).toBeInTheDocument()
    expect(screen.queryByText(/files$/)).toBeNull()
    expect(screen.queryByText('Quantization')).toBeNull()
  })

  it('shows disk trouble with the numbers', async () => {
    mockHub(
      result({
        disk: {
          status: 'insufficient',
          required_bytes: 4 * 1024 ** 3,
          free_bytes: 1024 ** 3,
          message:
            'The download is larger than the free disk space on this node.',
        },
      }),
    )
    renderPanel()
    expect(await screen.findByText('Disk too small')).toBeInTheDocument()
    expect(screen.getByText(/Needs 4.0 GiB, 1.0 GiB free/)).toBeInTheDocument()
  })

  it('shows an error state and still allows deploying', async () => {
    mockHub({ error: 'boom' }, 500)
    renderPanel()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      /You can still deploy/,
    )
  })

  it('sends the token but never renders it', async () => {
    const fetchMock = mockHub(
      result({ status: 'ok', gated: 'auto', access: 'granted' }),
    )
    renderPanel({ hfToken: 'hf_supersecret' })
    await screen.findByText('Found')
    expect(sentBody(fetchMock.mock.calls[0]?.[1])).toContain('hf_supersecret')
    expect(document.body.textContent).not.toContain('hf_supersecret')
  })
})
