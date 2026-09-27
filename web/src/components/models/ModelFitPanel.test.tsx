import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { INITIAL_DEPLOY_FORM } from '../../lib/modelDeployForm'
import { buildFitRequest } from '../../lib/modelFit'
import type { FitReport, NodeFit } from '../../types/modelFit'
import { ModelFitPanel } from './ModelFitPanel'

function node(over: Partial<NodeFit>): NodeFit {
  return {
    node_id: '',
    name: 'local',
    is_local: true,
    eligible: true,
    current: false,
    gpus: 1,
    total_bytes: 24 * 2 ** 30,
    reserved_bytes: 0,
    verdict: 'fits',
    weights_source: 'estimated',
    weights_bytes: 4.7 * 2 ** 30,
    kv_bytes: 1.2 * 2 ** 30,
    overhead_bytes: 0.8 * 2 ** 30,
    need_bytes: 6.7 * 2 ** 30,
    free_bytes: 8 * 2 ** 30,
    context_tokens: 8192,
    context_assumed: true,
    arithmetic:
      'weights 4.7 GiB + KV 1.2 GiB + overhead 0.8 GiB = 6.7 GiB of 8.0 GiB free',
    suggestions: [],
    ...over,
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('buildFitRequest', () => {
  it('needs a model reference', () => {
    expect(buildFitRequest(INITIAL_DEPLOY_FORM)).toBeNull()
  })

  it('maps the form to the request', () => {
    const req = buildFitRequest({
      ...INITIAL_DEPLOY_FORM,
      engine: 'vllm',
      model: ' org/m-8B ',
      gpus: '2',
      context: '4096',
      quantization: 'awq',
    })
    expect(req).toEqual({
      engine: 'vllm',
      model: 'org/m-8B',
      gpu_count: 2,
      context_length: 4096,
      quantization: 'awq',
    })
  })

  it('treats an unparsable GPU count as all', () => {
    expect(
      buildFitRequest({ ...INITIAL_DEPLOY_FORM, model: 'm:8b', gpus: 'x' })
        ?.gpu_count,
    ).toBe(-1)
  })
})

describe('ModelFitPanel', () => {
  it('shows a verdict per node and the arithmetic in the tip', async () => {
    const report: FitReport = {
      note: 'Real usage varies.',
      nodes: [
        node({}),
        node({
          node_id: 'n2',
          name: 'gpu-2',
          is_local: false,
          verdict: 'wont_fit',
        }),
      ],
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(report), { status: 200 }),
    )
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <ModelFitPanel
          form={{ ...INITIAL_DEPLOY_FORM, model: 'llama3.1:8b' }}
        />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('Fits')).toBeInTheDocument()
    expect(screen.getByText('Will not fit')).toBeInTheDocument()
    expect(screen.getByText(/Estimate only/)).toBeInTheDocument()
    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledTimes(1)
    })
  })

  it('renders nothing until a model is typed', () => {
    const client = new QueryClient()
    const { container } = render(
      <QueryClientProvider client={client}>
        <ModelFitPanel form={INITIAL_DEPLOY_FORM} />
      </QueryClientProvider>,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
