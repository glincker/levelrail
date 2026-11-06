import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StatusReasonTip } from './StatusReasonTip'

function renderTip(code: string | undefined) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <StatusReasonTip code={code} />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('StatusReasonTip', () => {
  it('shows a tip for a known reason', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify([
            {
              code: 'ReadinessFailed',
              kind: 'gate',
              meaning: 'never ready',
              next_step: 'check the probe',
            },
          ]),
        ),
      ),
    )
    renderTip('ReadinessFailed')
    expect(await screen.findByRole('button')).toBeTruthy()
  })

  it('renders nothing for an unknown reason', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('[]')))
    const { container } = renderTip('Whatever')
    await new Promise((r) => setTimeout(r, 20))
    expect(container.querySelector('button')).toBeNull()
  })
})
