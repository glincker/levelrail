import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DeployDetailI18n } from '../../test/deployDetailI18n'
import { DeployPhases } from './DeployPhases'

const body = {
  deploy_id: 'd1',
  status: 'running',
  elapsed_ms: 240_000,
  eta_ms: 180_000,
  typical_total_ms: 420_000,
  samples: 5,
  phases: [
    {
      phase: 'build',
      status: 'done',
      started_at: null,
      finished_at: null,
      duration_ms: 90_000,
      typical_ms: null,
    },
    {
      phase: 'export_image',
      status: 'running',
      started_at: null,
      finished_at: null,
      duration_ms: null,
      typical_ms: null,
    },
  ],
}

afterEach(() => vi.unstubAllGlobals())

describe('DeployPhases', () => {
  it('shows chips, the eta line and the quiet export note', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(JSON.stringify(body)))),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <DeployDetailI18n>
          <DeployPhases
            deployId="d1"
            running
            lastEventAt={Date.now() - 5 * 60_000}
          />
        </DeployDetailI18n>
      </QueryClientProvider>,
    )
    expect(await screen.findByText('Export image')).toBeInTheDocument()
    expect(
      screen.getByText('Usually takes about 7 min, 3 min left'),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Exporting the image can be quiet/),
    ).toBeInTheDocument()
  })
})
