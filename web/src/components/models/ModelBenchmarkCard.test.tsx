import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ModelsI18n } from '../../test/modelsI18n'
import type { CpuFit, ModelBenchmark } from '../../types/modelsCpu'
import { ModelBenchmarkCard } from './ModelBenchmarkCard'
import { createTestQueryClient } from '@/test/testQueryClient'

const bench: ModelBenchmark = {
  model: 'local',
  ran_at: '2026-10-16T12:00:00Z',
  duration_ms: 9000,
  prompts: [],
  prompt_tokens_per_sec: 88.4,
  gen_tokens_per_sec: 6.6,
  ttft_ms: 420,
  peak_memory_bytes: 3 * 2 ** 30,
  memory_limit_bytes: 4 * 2 ** 30,
  truncated: false,
  note: 'Measured.',
}
const estimate = { tokens_per_sec_low: 1.9, tokens_per_sec_high: 5.3 } as CpuFit

function renderCard(props: Partial<Parameters<typeof ModelBenchmarkCard>[0]>) {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <ModelsI18n>
        <ModelBenchmarkCard
          modelName="local"
          ready
          benchmark={undefined}
          estimate={estimate}
          {...props}
        />
      </ModelsI18n>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ModelBenchmarkCard', () => {
  it('shows the measured speed next to the estimate', () => {
    renderCard({ benchmark: bench })
    expect(screen.getByText('6.6 tokens/s')).toBeInTheDocument()
    expect(screen.getByText('1.9 to 5.3 tokens/s')).toBeInTheDocument()
    expect(screen.getByText('420 ms')).toBeInTheDocument()
    expect(
      screen.getByText(/Peak memory 3\.0 GB of 4\.0 GB/),
    ).toBeInTheDocument()
  })

  it('only shows the estimate before any run', () => {
    renderCard({})
    expect(
      screen.getByText(
        /Estimated 1\.9 to 5\.3 tokens per second\. Not measured yet\./,
      ),
    ).toBeInTheDocument()
  })

  it('disables the button for a model that is not loaded', () => {
    renderCard({ ready: false })
    expect(screen.getByRole('button', { name: 'Run benchmark' })).toBeDisabled()
    expect(screen.getByText(/never starts a stopped model/)).toBeInTheDocument()
  })

  it('runs the benchmark only when asked', async () => {
    const spy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify(bench)))
    renderCard({})
    expect(spy).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Run benchmark' }))
    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith('/api/v1/models/local/benchmark', {
        method: 'POST',
      })
    })
  })
})
