import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { jsonResponse } from '../../test/newSurfacesI18n'
import { ImageRemoveDialog } from './ImageRemoveDialog'

const img = (id: string, tag: string) => ({
  id,
  tags: [tag],
  size_bytes: 10 * 1024 * 1024,
  shared_bytes: 0,
  containers: 0,
  created_at: '2026-10-01T00:00:00Z',
  apps: [],
  protected: false,
  unmanaged: false,
  in_use: false,
  removable: true,
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function bodies(
  fetchMock: ReturnType<typeof vi.fn>,
): Record<string, unknown>[] {
  return fetchMock.mock.calls.map(
    (c) =>
      JSON.parse((c[1] as RequestInit).body as string) as Record<
        string,
        unknown
      >,
  )
}

describe('ImageRemoveDialog', () => {
  it('previews with dry_run, then removes for real and summarizes a failure', async () => {
    const a = img('sha256:aaa', 'web:a')
    const b = img('sha256:bbb', 'web:b')
    const fetchMock = vi.fn((_url: string, init: RequestInit) => {
      const dry = (JSON.parse(init.body as string) as { dry_run: boolean })
        .dry_run
      return Promise.resolve(
        jsonResponse(
          dry
            ? {
                dry_run: true,
                candidates: [a, b],
                removed: [],
                failed: [],
                reclaimed_bytes: 20971520,
              }
            : {
                dry_run: false,
                candidates: [],
                removed: [a.id],
                failed: [{ id: b.id, error: 'image is being pulled' }],
                reclaimed_bytes: 10485760,
              },
        ),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    const onRemoved = vi.fn()
    render(
      <NewSurfacesProviders>
        <ImageRemoveDialog
          node="local"
          open
          onOpenChange={vi.fn()}
          ids={[a.id, b.id]}
          allowUnmanaged={false}
          onRemoved={onRemoved}
        />
      </NewSurfacesProviders>,
    )
    expect(await screen.findByText('web:a')).toBeInTheDocument()
    expect(screen.getByText(/downloaded or rebuilt again/u)).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Remove 2 images' }),
    )
    expect(
      await screen.findByText(/Removed 1 image, freed/u),
    ).toBeInTheDocument()
    expect(screen.getByText(/image is being pulled/u)).toBeInTheDocument()
    expect(bodies(fetchMock)).toEqual([
      {
        node: 'local',
        ids: [a.id, b.id],
        dry_run: true,
        include_unmanaged_unused: false,
      },
      {
        node: 'local',
        ids: [a.id, b.id],
        dry_run: false,
        include_unmanaged_unused: false,
      },
    ])
    expect(onRemoved).toHaveBeenCalledWith([a.id])
  })

  it('re-previews with include_unmanaged_unused when the switch is on', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        jsonResponse({
          dry_run: true,
          candidates: [],
          removed: [],
          failed: [],
          reclaimed_bytes: 0,
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <NewSurfacesProviders>
        <ImageRemoveDialog
          node="local"
          open
          onOpenChange={vi.fn()}
          ids={['sha256:aaa']}
          allowUnmanaged
          onRemoved={vi.fn()}
        />
      </NewSurfacesProviders>,
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    await userEvent.click(
      screen.getByRole('switch', { name: /Also remove other images/u }),
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
    expect(bodies(fetchMock)[1]).toMatchObject({
      dry_run: true,
      include_unmanaged_unused: true,
    })
  })
})
