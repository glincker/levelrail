import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BrowseTemplatesFields } from './BrowseTemplatesFields'

function fakeJsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderFields() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <BrowseTemplatesFields open onCreated={() => {}} />
    </QueryClientProvider>,
  )
}

describe('BrowseTemplatesFields', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows a RAM advisory badge only for templates that declare one', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse([
            {
              id: 'ollama-mistral',
              name: 'Ollama: Mistral 7B',
              slogan: "Mistral's 7B instruct model.",
              category: 'AI',
              documentation_url: 'https://ollama.com/library/mistral',
              recommended_memory_bytes: 6 * 1024 * 1024 * 1024,
            },
            {
              id: 'n8n',
              name: 'n8n',
              slogan: 'Build automations.',
              category: 'Automation',
              documentation_url: 'https://docs.n8n.io',
            },
          ]),
        ),
      ),
    )

    renderFields()

    expect(
      await screen.findByText('~6.0 GiB RAM recommended'),
    ).toBeInTheDocument()
    expect(await screen.findByText('n8n')).toBeInTheDocument()
    expect(screen.getAllByText(/RAM recommended/)).toHaveLength(1)
  })
})
