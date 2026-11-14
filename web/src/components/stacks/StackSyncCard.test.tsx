import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { StackSyncCard } from './StackSyncCard'
import { jsonResponse, renderWithProviders } from './stacksTestUtils'
import type { StackSyncInfo } from '../../queries/stackSync'

afterEach(() => {
  vi.unstubAllGlobals()
})

const cfg = {
  repo_url: 'https://git.example.com/infra.git',
  branch: 'main',
  path: 'stacks/shop',
  webhook_only: false,
  enabled: true,
  apply: false,
  prune: false,
  deploy: false,
}

function mock(
  info: StackSyncInfo,
  extra?: (url: string, init?: RequestInit) => Response | undefined,
) {
  const fn = vi.fn((url: string, init?: RequestInit) => {
    const r = extra?.(url, init)
    if (r) return Promise.resolve(r)
    return Promise.resolve(jsonResponse(info))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderCard(requireApproval = false) {
  return renderWithProviders(
    <StackSyncCard name="shop" isRoot requireApproval={requireApproval} />,
  )
}

describe('StackSyncCard', () => {
  it('shows never synced when there is no status', async () => {
    mock({ config: null, webhook_secret_set: false })
    renderCard()
    expect(await screen.findByText('Never synced')).toBeTruthy()
    expect(screen.getByText('No secret yet')).toBeTruthy()
  })

  it('shows a pending plan with counts and a view plan action', async () => {
    mock({
      config: cfg,
      webhook_secret_set: true,
      status: {
        pending: true,
        commit: 'abcdef1234',
        checked_at: '2026-11-01T00:00:00Z',
        counts: { create: 2, update: 1, delete: 0 },
        revision: 4,
      },
    })
    renderCard(true)
    expect(await screen.findByText('Pending plan')).toBeTruthy()
    expect(screen.getByText('Commit abcdef1')).toBeTruthy()
    expect(screen.getByText('2 create')).toBeTruthy()
    expect(screen.getByText(/never approves itself/)).toBeTruthy()
  })

  it('shows an error code badge with a human sentence', async () => {
    mock({
      config: cfg,
      webhook_secret_set: true,
      status: {
        pending: false,
        code: 'sync_repo_unreachable',
        checked_at: '2026-11-01T00:00:00Z',
      },
    })
    renderCard()
    expect(await screen.findByText('sync_repo_unreachable')).toBeTruthy()
    expect(screen.getByText(/could not be fetched/)).toBeTruthy()
  })

  it('disables prune and deploy until apply is on, and PUT sends booleans', async () => {
    const fn = mock({ config: cfg, webhook_secret_set: true }, (_u, init) =>
      init?.method === 'PUT'
        ? jsonResponse({ config: cfg, webhook_secret_set: true })
        : undefined,
    )
    renderCard()
    const prune = await screen.findByRole('switch', { name: 'Prune' })
    const deploy = screen.getByRole('switch', { name: 'Deploy' })
    expect(prune.hasAttribute('data-disabled')).toBe(true)
    expect(deploy.hasAttribute('data-disabled')).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(fn.mock.calls.some(([, i]) => i?.method === 'PUT')).toBe(true)
    })
    const put = fn.mock.calls.find(([, i]) => i?.method === 'PUT')
    const body = JSON.parse(put?.[1]?.body as string) as Record<string, unknown>
    expect(body.apply).toBe(false)
    expect(body.prune).toBe(false)
    expect(body.deploy).toBe(false)
    fireEvent.click(screen.getByRole('switch', { name: 'Apply' }))
    await waitFor(() => {
      expect(
        screen
          .getByRole('switch', { name: 'Prune' })
          .hasAttribute('data-disabled'),
      ).toBe(false)
    })
  })

  it('shows the cooldown message after a 429 refresh', async () => {
    mock(
      { config: cfg, webhook_secret_set: true, refresh_cooldown_seconds: 45 },
      (u, init) =>
        u.endsWith('/refresh') && init?.method === 'POST'
          ? new Response(
              JSON.stringify({ error: 'a sync refresh ran recently' }),
              {
                status: 429,
                headers: { 'Content-Type': 'application/json' },
              },
            )
          : undefined,
    )
    renderCard()
    fireEvent.click(await screen.findByRole('button', { name: /Refresh now/ }))
    expect(await screen.findByText(/ran recently/)).toBeTruthy()
    expect(
      screen
        .getByRole('button', { name: /Refresh now/ })
        .hasAttribute('disabled'),
    ).toBe(true)
  })

  it('keeps refresh enabled after a 429 when the server sent no cooldown', async () => {
    mock({ config: cfg, webhook_secret_set: true }, (u, init) =>
      u.endsWith('/refresh') && init?.method === 'POST'
        ? new Response(JSON.stringify({ error: 'ran recently' }), {
            status: 429,
            headers: { 'Content-Type': 'application/json' },
          })
        : undefined,
    )
    renderCard()
    fireEvent.click(await screen.findByRole('button', { name: /Refresh now/ }))
    expect(await screen.findByText(/ran recently/)).toBeTruthy()
    expect(
      screen
        .getByRole('button', { name: /Refresh now/ })
        .hasAttribute('disabled'),
    ).toBe(false)
  })

  it('shows the rotated secret once and removes it from the DOM after close', async () => {
    mock({ config: cfg, webhook_secret_set: false }, (u, init) =>
      u.endsWith('/rotate') && init?.method === 'POST'
        ? jsonResponse({
            secret: 'whsec_topsecret',
            sync_url: 'https://lr.example.com/hook/sync',
            refresh_url: 'https://lr.example.com/hook/refresh',
          })
        : undefined,
    )
    renderCard()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Rotate webhook secret' }),
    )
    expect(screen.getByText(/old secret stops working/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Rotate' }))
    const field = await screen.findByDisplayValue('whsec_topsecret')
    expect(field).toBeTruthy()
    expect(screen.getByText(/will not be shown again/)).toBeTruthy()
    expect(
      screen.getByDisplayValue('https://lr.example.com/hook/sync'),
    ).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))
    await waitFor(() => {
      expect(screen.queryByDisplayValue('whsec_topsecret')).toBeNull()
    })
    expect(document.body.textContent).not.toContain('whsec_topsecret')
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Rotate webhook secret' }),
      ).toHaveFocus()
    })
  })
})
