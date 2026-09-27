import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { UpgradePreflight } from './UpgradePreflight'
import type { UpdatePreflight } from '../../queries/updates'

const base: UpdatePreflight = {
  current_version: 'v1.0.0',
  latest_version: 'v1.1.0',
  update_available: true,
  release_url: null,
  release_notes: 'Fixes things',
  checks: [
    {
      code: 'disk_space',
      name: 'Free disk space',
      status: 'ok',
      message: '9 GiB free',
    },
  ],
  blocked: false,
  upgrade_command: 'sh -s upgrade',
  rollback_command: 'restore-snapshot --list',
}

function renderWith(data: UpdatePreflight) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(data),
      }),
    ),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <UpgradePreflight />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('UpgradePreflight', () => {
  it('shows checks and the copyable command when an update is available', async () => {
    renderWith(base)
    expect(await screen.findByText('Free disk space')).toBeInTheDocument()
    expect(screen.getByText('Pass')).toBeInTheDocument()
    expect(screen.getByText('sh -s upgrade')).toBeInTheDocument()
    expect(screen.getByText('Fixes things')).toBeInTheDocument()
  })

  it('hides the command and explains when blocked', async () => {
    renderWith({
      ...base,
      blocked: true,
      checks: [
        {
          code: 'docker_engine',
          name: 'Docker Engine version',
          status: 'fail',
          message: 'known bad',
        },
      ],
    })
    expect(await screen.findByText('Blocked')).toBeInTheDocument()
    expect(screen.getByText(/Upgrade blocked/)).toBeInTheDocument()
    expect(screen.queryByText('sh -s upgrade')).not.toBeInTheDocument()
  })
})
