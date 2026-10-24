import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderInRouter } from '../../test/renderInRouter'
import type { CleanupOverview, CleanupTarget } from '../../types/cleanup'
import { CleanupHistoryTab } from './CleanupHistoryTab'
import { CleanupOverviewTab } from './CleanupOverviewTab'

const rootState = vi.hoisted(() => ({ root: true }))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => rootState.root }))

const MB = 1024 * 1024

function target(over: Partial<CleanupTarget>): CleanupTarget {
  return {
    id: 'audit.log',
    category: 'audit',
    title: 'Audit log',
    description: 'Security and admin events.',
    policy_key: 'retention.audit_log_days',
    days: 365,
    mode: 'on',
    origin: 'existing',
    destructive: true,
    floor: 90,
    usage: { rows: 12403, bytes: 38 * MB, oldest: '2026-01-01T00:00:00Z' },
    last_run: null,
    next_run: null,
    ...over,
  }
}

const overview: CleanupOverview = {
  scheduler: { enabled: true, interval: '6h' },
  targets: [
    target({}),
    target({
      id: 'auth.sessions',
      category: 'auth',
      title: 'Expired sessions',
      policy_key: 'retention.sessions_days',
      origin: 'new',
      mode: 'preview',
      destructive: false,
      floor: 0,
      days: 30,
      usage: { rows: 90, bytes: 2 * MB },
    }),
    target({
      id: 'janitor.containers',
      category: 'docker',
      title: 'Unused containers',
      policy_key: '',
      usage: { rows: 3, bytes: 10 * MB },
    }),
  ],
}

let calls: { url: string; init?: RequestInit }[] = []

beforeEach(() => {
  rootState.root = true
  calls = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init })
      let body: unknown = {}
      if (url === '/api/v1/cleanup') body = overview
      else if (url.includes('settings/runtime'))
        body = { settings: [], restart: { supported: false } }
      else if (url.includes('/preview')) body = { rows: 12403, bytes: 38 * MB }
      else if (url.endsWith('/run')) body = { rows: 12403, bytes: 38 * MB }
      else if (url.includes('/cleanup/runs'))
        body = { runs: [], next_cursor: '' }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      })
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('CleanupOverviewTab', () => {
  it('sums usage in the hero and flags preview-mode targets', async () => {
    await renderInRouter(<CleanupOverviewTab />)
    expect(await screen.findByText('Audit log')).toBeInTheDocument()
    expect(screen.getByText('50 MiB')).toBeInTheDocument()
    expect(
      screen.getByText('Preview mode, nothing is deleted yet'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('1 data type is in preview mode'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Open Disk and Docker/ }),
    ).toBeInTheDocument()
  })

  it('shows the inline impact summary and the previewed count in the confirm dialog', async () => {
    await renderInRouter(<CleanupOverviewTab />)
    await screen.findByText('Audit log')
    const row = document.querySelector(
      '[data-target="audit.log"]',
    ) as HTMLElement
    const inRow = within(row)
    await userEvent.click(inRow.getByRole('button', { name: 'Preview' }))
    expect(
      await screen.findByText(
        'Will remove 12,403 rows, about 38 MiB, older than 365 days.',
      ),
    ).toBeInTheDocument()

    await userEvent.click(inRow.getByRole('button', { name: 'Clean now' }))
    expect(
      await screen.findByText(
        'This permanently removes 12,403 rows, about 38 MiB, older than 365 days.',
      ),
    ).toBeInTheDocument()
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Clean now',
      }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.url.endsWith('/audit.log/run'))).toBe(true)
    })
    expect(
      await screen.findByText(
        /Removed 12,403 rows, 38 MiB. Deletion is permanent./,
      ),
    ).toBeInTheDocument()
  })

  it('disables every control and explains why for a non-root user', async () => {
    rootState.root = false
    await renderInRouter(<CleanupOverviewTab />)
    await screen.findByText('Audit log')
    expect(screen.getByText(/need a root account/)).toBeInTheDocument()
    const row = document.querySelector(
      '[data-target="audit.log"]',
    ) as HTMLElement
    for (const b of row.querySelectorAll('button')) expect(b).toBeDisabled()
  })
})

describe('CleanupHistoryTab', () => {
  it('links the exports to the runs endpoint with the format', async () => {
    await renderInRouter(<CleanupHistoryTab />)
    const csv = screen.getByRole('link', { name: /Export CSV/ })
    const json = screen.getByRole('link', { name: /Export JSON/ })
    expect(csv).toHaveAttribute('href', '/api/v1/cleanup/runs?format=csv')
    expect(json).toHaveAttribute('href', '/api/v1/cleanup/runs?format=json')
    expect(await screen.findByText('No runs match')).toBeInTheDocument()
  })
})
