import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { jsonResponse } from '../../test/newSurfacesI18n'
import { ImageCleanupDialog } from './ImageCleanupDialog'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const candidate = {
  id: 'sha256:abcdef1234567890',
  tags: ['web:old'],
  size_bytes: 10 * 1024 * 1024,
  shared_bytes: 0,
  containers: 0,
  created_at: '2026-10-01T00:00:00Z',
  apps: [],
  protected: false,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ImageCleanupDialog', () => {
  it('lists candidates and runs a real cleanup on confirm', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        jsonResponse({
          dry_run: false,
          candidates: [],
          removed: [candidate.id],
          failed: [],
          reclaimed_bytes: candidate.size_bytes,
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <NewSurfacesProviders>
        <ImageCleanupDialog
          node="local"
          open
          onOpenChange={vi.fn()}
          candidates={[candidate]}
          keepPerApp={3}
          reclaimBytes={candidate.size_bytes}
        />
      </NewSurfacesProviders>,
    )
    expect(screen.getByText('web:old')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /^Free /u }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(url).toBe('/api/v1/images/gc')
    expect(JSON.parse(init.body as string)).toEqual({
      node: 'local',
      dry_run: false,
    })
  })
})
