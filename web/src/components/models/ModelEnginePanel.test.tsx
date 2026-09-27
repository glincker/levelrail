import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { EngineMetricsReport } from '../../types/models'
import { ModelEnginePanel } from './ModelEnginePanel'

function series(
  id: string,
  label: string,
  unit: EngineMetricsReport['series'][number]['unit'],
  supported: boolean,
  latest: number | null,
) {
  return {
    id,
    label,
    unit,
    supported,
    latest,
    points:
      latest === null
        ? []
        : [
            { t: '2026-09-26T10:00:00Z', v: latest - 1 },
            { t: '2026-09-26T10:00:15Z', v: latest },
          ],
  }
}

function report(over: Partial<EngineMetricsReport>): EngineMetricsReport {
  return {
    model: 'chat',
    engine: 'vllm',
    from: '2026-09-26T09:00:00Z',
    to: '2026-09-26T10:00:00Z',
    collecting: true,
    note: 'scraped',
    health: { state: 'ok', summary: 'Engine looks healthy.', reasons: [] },
    series: [],
    ...over,
  }
}

function renderPanel(rep: EngineMetricsReport) {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(rep), { status: 200 }),
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ModelEnginePanel modelName="chat" engine={rep.engine} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ModelEnginePanel', () => {
  it('shows values and a healthy pill for vLLM', async () => {
    renderPanel(
      report({
        series: [
          series('kv_cache', 'KV cache usage', 'percent', true, 42),
          series('ttft', 'Time to first token', 'seconds', true, 0.25),
        ],
      }),
    )
    expect(await screen.findByText('Engine healthy')).toBeInTheDocument()
    expect(screen.getByText('KV cache usage')).toBeInTheDocument()
    expect(screen.getByText('250')).toBeInTheDocument()
    expect(screen.getByText('ms')).toBeInTheDocument()
  })

  it('marks metrics an engine cannot expose as not available', async () => {
    renderPanel(
      report({
        engine: 'ollama',
        collecting: false,
        health: {
          state: 'unknown',
          summary: 'No engine samples yet.',
          reasons: [],
        },
        series: [
          series('kv_cache', 'KV cache usage', 'percent', false, null),
          series('queue', 'Queued requests', 'count', false, null),
          series('vram', 'VRAM in use by the model', 'bytes', true, null),
          series('cpu_offload', 'Share running on CPU', 'percent', false, null),
        ],
      }),
    )
    expect(await screen.findByText('No data yet')).toBeInTheDocument()
    expect(screen.getAllByText('Not available for ollama')).toHaveLength(2)
    expect(screen.queryByText('Share running on CPU')).not.toBeInTheDocument()
    expect(screen.getByText('no data')).toBeInTheDocument()
  })

  it('lists warning reasons', async () => {
    renderPanel(
      report({
        health: {
          state: 'warn',
          summary: 'KV cache is 97% full.',
          reasons: ['KV cache is 97% full.'],
        },
        series: [series('kv_cache', 'KV cache usage', 'percent', true, 97)],
      }),
    )
    expect(await screen.findByText('Needs attention')).toBeInTheDocument()
    expect(screen.getByText('KV cache is 97% full.')).toBeInTheDocument()
  })
})
