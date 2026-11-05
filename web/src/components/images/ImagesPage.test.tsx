import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { jsonResponse } from '../../test/newSurfacesI18n'
import { ImagesSection } from './ImagesPage'

const root = vi.hoisted(() => ({ value: true }))

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => root.value }))
vi.mock('./DockerfileLintCard', () => ({ DockerfileLintCard: () => null }))
vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const base = {
  shared_bytes: 0,
  containers: 0,
  created_at: '2026-10-01T00:00:00Z',
  apps: [],
  protected: false,
  in_use: false,
  unmanaged: false,
  removable: true,
}
const images = [
  { ...base, id: 'sha256:aaa', tags: ['web:old'], size_bytes: 3000 },
  {
    ...base,
    id: 'sha256:bbb',
    tags: ['nginx:1'],
    size_bytes: 2000,
    unmanaged: true,
  },
  {
    ...base,
    id: 'sha256:ccc',
    tags: ['db:live'],
    size_bytes: 1000,
    in_use: true,
    protected: true,
    removable: false,
    blocked_reason: 'used by a container',
  },
]

function stub() {
  const fetchMock = vi.fn(() =>
    Promise.resolve(
      jsonResponse({
        node: '',
        images,
        totals: {
          count: 3,
          size_bytes: 6000,
          reclaimable_bytes: 3000,
          protected_count: 1,
          unmanaged_unused_count: 1,
          unmanaged_unused_bytes: 2000,
          app_images: 2,
          other_images: 1,
        },
        retention: { keep_per_app: 3, min_age_hours: 24 },
        signatures: { mode: 'off', configured: false },
      }),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
}

function renderPage() {
  render(
    <NewSurfacesProviders>
      <ImagesSection node="" gcOpen={false} onGcOpenChange={() => undefined} />
    </NewSurfacesProviders>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  root.value = true
})

describe('ImagesSection', () => {
  it('disables blocked rows with the reason and shows the action bar on selection', async () => {
    stub()
    renderPage()
    const blocked = await screen.findByRole('checkbox', {
      name: 'Select db:live',
    })
    expect(blocked).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByTitle('used by a container')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('checkbox', { name: 'Select web:old' }),
    )
    const bar = screen.getByRole('region', { name: 'Selected images' })
    expect(within(bar).getByText(/1 selected/u)).toBeInTheDocument()
    await userEvent.click(within(bar).getByRole('button', { name: 'Clear' }))
    expect(screen.queryByRole('region', { name: 'Selected images' })).toBeNull()
  })

  it('filters by chip', async () => {
    stub()
    renderPage()
    await screen.findByText('web:old')
    await userEvent.click(screen.getByRole('button', { name: 'Other' }))
    expect(screen.queryByText('web:old')).toBeNull()
    expect(screen.getByText('nginx:1')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'In use' }))
    expect(screen.getByText('db:live')).toBeInTheDocument()
    expect(screen.queryByText('nginx:1')).toBeNull()
  })

  it('hides mutating controls for a non-root account', async () => {
    root.value = false
    stub()
    renderPage()
    await screen.findByText('web:old')
    expect(screen.queryByRole('checkbox')).toBeNull()
    expect(
      screen.queryByRole('button', { name: 'Review and clean up' }),
    ).toBeNull()
    expect(screen.getByText(/Only the root account/u)).toBeInTheDocument()
  })
})
