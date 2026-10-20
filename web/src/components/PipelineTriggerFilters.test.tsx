import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PipelineTriggerFilters } from './PipelineTriggerFilters'
import type { PipelineFilters } from '../types/pipelines'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const base: PipelineFilters = {
  paths: ['src/**'],
  paths_ignore: [],
  report_status: true,
  split: false,
}

function renderPanel(
  filters: PipelineFilters,
  onApply: (next: string) => void,
  yaml = 'version: 1\n',
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PipelineTriggerFilters yaml={yaml} filters={filters} onApply={onApply} />
    </QueryClientProvider>,
  )
}

describe('PipelineTriggerFilters', () => {
  let bodies: Record<string, unknown>[]

  beforeEach(() => {
    bodies = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: RequestInfo | URL, init?: RequestInit) => {
        bodies.push(JSON.parse(init?.body as string) as Record<string, unknown>)
        return Promise.resolve(jsonResponse({ yaml: 'rewritten\n' }))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends the edited globs and hands back the rewritten YAML', async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    renderPanel(base, onApply)
    await user.type(
      screen.getByRole('textbox', { name: /Skip when only these change/ }),
      '**/*.md',
    )
    await user.click(screen.getByRole('button', { name: 'Apply to YAML' }))
    await waitFor(() => expect(onApply).toHaveBeenCalledWith('rewritten\n'))
    expect(bodies[0]).toMatchObject({
      paths: ['src/**'],
      paths_ignore: ['**/*.md'],
      report_status: true,
    })
  })

  it('leaves distinct push and pull request filters alone', async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    renderPanel({ ...base, split: true }, onApply)
    expect(
      screen.getByRole('textbox', { name: /Only run for changes to/ }),
    ).toHaveProperty('disabled', true)
    await user.click(screen.getByRole('switch'))
    await user.click(screen.getByRole('button', { name: 'Apply to YAML' }))
    await waitFor(() => expect(onApply).toHaveBeenCalled())
    expect(bodies[0]).toEqual({ yaml: 'version: 1\n', report_status: false })
  })

  it('drops a response when the YAML changed while it was applying', async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    let release: () => void = () => undefined
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            release = () => resolve(jsonResponse({ yaml: 'stale\n' }))
          }),
      ),
    )
    const view = renderPanel(base, onApply)
    await user.click(screen.getByRole('switch'))
    await user.click(screen.getByRole('button', { name: 'Apply to YAML' }))

    const queryClient = new QueryClient()
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <PipelineTriggerFilters
          yaml={'version: 1\n# typed meanwhile\n'}
          filters={base}
          onApply={onApply}
        />
      </QueryClientProvider>,
    )
    release()
    await waitFor(() => expect(onApply).not.toHaveBeenCalled())
    await new Promise((r) => setTimeout(r, 20))
    expect(onApply).not.toHaveBeenCalled()
  })
})
