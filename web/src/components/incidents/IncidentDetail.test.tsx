import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClientProvider } from '@tanstack/react-query'
import { IncidentDetail } from './IncidentDetail'
import type { Incident } from '../../types/incident'
import { createTestQueryClient } from '@/test/testQueryClient'

const base: Incident = {
  id: 'si_1',
  title: 'Site down',
  status: 'investigating',
  impact: 'none',
  source: 'monitor',
  internal_state: 'triggered',
  published: false,
  updated_at: '2026-10-29T10:00:00Z',
  starts_at: '2026-10-29T09:00:00Z',
  draft: { title: 'Site down', body: 'We are looking into it.' },
}

function mockApi(afterSave: Incident) {
  let saved = false
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push(`${method} ${url}`)
      if (method === 'PUT') {
        saved = true
        return Promise.resolve(Response.json({ id: 'si_1' }))
      }
      if (method === 'POST') return Promise.resolve(Response.json({}))
      return Promise.resolve(Response.json(saved ? afterSave : base))
    }),
  )
  return calls
}

async function publishFlow() {
  const qc = createTestQueryClient()
  render(
    <QueryClientProvider client={qc}>
      <IncidentDetail id="si_1" />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'incidents.publish.action' }),
  )
  await user.click(
    await screen.findByRole('button', { name: 'incidents.publish.confirm' }),
  )
}

describe('IncidentDetail publish', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('does not publish when another operator edited the draft after the save', async () => {
    const calls = mockApi({
      ...base,
      updated_at: '2026-10-29T10:05:00Z',
      draft: { title: 'Site down', body: 'Someone else wrote this.' },
    })
    await publishFlow()
    await waitFor(() => {
      expect(calls.some((c) => c.startsWith('PUT'))).toBe(true)
    })
    await new Promise((r) => setTimeout(r, 50))
    expect(calls.some((c) => c.includes('/publish'))).toBe(false)
  })

  it('publishes when the fetched draft matches the confirmed text', async () => {
    const calls = mockApi({ ...base, updated_at: '2026-10-29T10:05:00Z' })
    await publishFlow()
    await waitFor(() => {
      expect(calls.some((c) => c.includes('/publish'))).toBe(true)
    })
  })

  it('shows a publish failure inside the dialog and keeps it open', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          return Promise.resolve(
            Response.json({ error: 'save exploded' }, { status: 500 }),
          )
        }
        return Promise.resolve(Response.json(base))
      }),
    )
    await publishFlow()
    expect(await screen.findAllByText(/save exploded/)).not.toHaveLength(0)
    expect(
      screen.getByRole('button', { name: 'incidents.publish.confirm' }),
    ).toBeInTheDocument()
  })
})
