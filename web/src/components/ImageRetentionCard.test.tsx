import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../test/NewSurfacesProviders'
import { jsonResponse } from '../test/newSurfacesI18n'
import { ImageRetentionCard } from './ImageRetentionCard'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ImageRetentionCard', () => {
  it('previews removals before allowing a save', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url.endsWith('/preview')) {
        return Promise.resolve(
          jsonResponse({
            app: 'web',
            keep: 2,
            effective_keep: 2,
            dry_run: true,
            remove: [
              {
                id: 'sha256:abcdef1234567890',
                tags: ['web:old'],
                size_bytes: 2048,
                reclaim_bytes: 2048,
                created_at: '2026-10-01T00:00:00Z',
              },
            ],
            remove_count: 1,
            reclaim_bytes: 2048,
            kept: [],
          }),
        )
      }
      const keep = init?.method === 'PUT' ? 2 : 0
      return Promise.resolve(
        jsonResponse({
          app: 'web',
          keep,
          effective_keep: keep || 3,
          source: keep ? 'app' : 'global',
          global_keep: 3,
          min_keep: 2,
        }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    render(
      <NewSurfacesProviders>
        <ImageRetentionCard appName="web" />
      </NewSurfacesProviders>,
    )
    const input = await screen.findByRole('spinbutton')
    const save = screen.getByRole('button', { name: /save/i })
    await user.clear(input)
    await user.type(input, '2')
    expect(save).toBeDisabled()
    await user.click(screen.getByRole('button', { name: /preview/i }))
    expect(await screen.findByText('web:old')).toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'PUT')).toBe(
      false,
    )
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)
    await waitFor(() =>
      expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'PUT')).toBe(
        true,
      ),
    )
  })
})
