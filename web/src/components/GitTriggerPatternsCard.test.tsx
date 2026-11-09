import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GitTriggerPatternsCard } from './GitTriggerPatternsCard'
import { AllI18n } from '../test/allI18n'
import { createTestQueryClient } from '@/test/testQueryClient'

function wrap(ui: ReactNode) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <AllI18n>{ui}</AllI18n>
    </QueryClientProvider>,
  )
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function source(extra: Record<string, unknown>) {
  return {
    app: 'shop',
    repo_url: 'https://git.example.test/acme/shop.git',
    branch: 'main',
    ...extra,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GitTriggerPatternsCard', () => {
  it('edits tag patterns in release mode and sends tag_pattern', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'PUT'
          ? jsonResponse({ tag_pattern: 'v*' })
          : jsonResponse(source({ trigger_mode: 'release', tag_pattern: '' })),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    wrap(<GitTriggerPatternsCard appName="shop" />)

    const field = await screen.findByLabelText('Tags that deploy')
    const save = screen.getByRole('button', { name: 'Save patterns' })
    expect(save).toBeDisabled()

    await user.type(field, 'v*')
    expect(save).toBeEnabled()
    await user.click(save)

    await waitFor(() => {
      const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT')
      expect(put).toBeTruthy()
      expect(String(put?.[0])).toContain('/git-source/deploy-settings')
      expect(JSON.parse((put?.[1] as RequestInit).body as string)).toEqual({
        tag_pattern: 'v*',
      })
    })
  })

  it('edits branch patterns in push mode and sends branch_pattern', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'PUT'
          ? jsonResponse({ branch_pattern: 'release/*' })
          : jsonResponse(
              source({ trigger_mode: 'push', branch_pattern: 'main' }),
            ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    wrap(<GitTriggerPatternsCard appName="shop" />)

    const field = await screen.findByLabelText('Branches that deploy')
    expect(field).toHaveValue('main')

    await user.clear(field)
    await user.type(field, 'release/*')
    await user.click(screen.getByRole('button', { name: 'Save patterns' }))

    await waitFor(() => {
      const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT')
      expect(JSON.parse((put?.[1] as RequestInit).body as string)).toEqual({
        branch_pattern: 'release/*',
      })
    })
  })

  it('renders nothing when the app has no git source', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse({ error: 'not found' }, 404))),
    )
    wrap(<GitTriggerPatternsCard appName="shop" />)
    await waitFor(() =>
      expect(screen.queryByLabelText('Tags that deploy')).toBeNull(),
    )
    expect(screen.queryByLabelText('Branches that deploy')).toBeNull()
  })
})
