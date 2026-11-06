import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { StorageHubPage } from './StorageHubPage'

const GIB = 1024 ** 3

const state = vi.hoisted(() => ({
  role: 'admin',
  status: {
    data: undefined,
    isPending: false,
    isError: false,
    refetch: () => Promise.resolve(),
  },
  images: { data: undefined },
  inventory: { data: undefined },
}))

vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({
    role: state.role,
    can: () => state.role === 'admin',
  }),
}))
vi.mock('@/hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('../../queries/systemStatus', () => ({
  useSystemStatusOptional: () => state.status,
}))
vi.mock('../../queries/imageInventory', () => ({
  useImageInventory: () => state.images,
}))
vi.mock('../../queries/storageInventory', () => ({
  useStorageInventory: () => state.inventory,
}))
vi.mock('../../queries/nodes', () => ({
  useNodeListOptional: () => ({ data: [] }),
}))
vi.mock('../../queries/runtimeSettings', () => ({
  useRuntimeSettings: () => ({
    data: {
      settings: [
        { key: 'disk.low_free_mb', value: '2048' },
        { key: 'disk.critical_free_mb', value: '512' },
      ],
    },
  }),
}))
vi.mock('../../queries/diskHistory', () => ({
  useDiskHistory: () => ({
    data: { samples: [], forecast: { days_until_full: 38, basis_days: 10 } },
  }),
}))
vi.mock('../shell/PageTemplate', () => ({
  PageTemplate: ({
    children,
    actions,
  }: {
    children: ReactNode
    actions?: ReactNode
  }) => (
    <div>
      {actions}
      {children}
    </div>
  ),
}))
vi.mock('./StorageTrend', () => ({ StorageTrend: () => <p>trend</p> }))
vi.mock('./StoragePolicyStrip', () => ({
  StoragePolicyStrip: () => <p>policy</p>,
}))
vi.mock('./StorageItemList', () => ({
  StorageItemList: (p: { view?: string; filter?: string }) => (
    <p>{`list view=${p.view ?? 'type'} filter=${p.filter ?? 'none'}`}</p>
  ),
}))
vi.mock('./CleanupPlanDrawer', () => ({
  CleanupPlanDrawer: ({ open }: { open: boolean }) =>
    open ? <p>drawer open</p> : null,
}))
vi.mock('../images/DockerHealthCard', () => ({ DockerHealthCard: () => null }))
vi.mock('../images/SignaturePolicyCard', () => ({
  SignaturePolicyCard: () => null,
}))

const usage = {
  images_total_bytes: 9 * GIB,
  images_reclaimable_bytes: 0,
  containers_total_bytes: 2 * GIB,
  containers_reclaimable_bytes: 0,
  volumes_total_bytes: 3 * GIB,
  volumes_reclaimable_bytes: 0,
  build_cache_total_bytes: 4 * GIB,
  build_cache_reclaimable_bytes: 2 * GIB,
}

function setup(over: Partial<typeof state> = {}) {
  state.role = over.role ?? 'admin'
  state.status = over.status ?? {
    data: {
      data_dir_total_bytes: 100 * GIB,
      data_dir_free_bytes: 38 * GIB,
      docker_connected: true,
      docker_disk_usage: usage,
    },
    isPending: false,
    isError: false,
    refetch: () => Promise.resolve(),
  }
  state.images = { data: undefined }
  state.inventory = { data: undefined }
}

function renderHub(props: Partial<Parameters<typeof StorageHubPage>[0]> = {}) {
  const handlers = {
    onFilterChange: vi.fn(),
    onViewChange: vi.fn(),
    onDrawerChange: vi.fn(),
  }
  render(
    <AllI18n>
      <StorageHubPage drawerOpen={false} {...handlers} {...props} />
    </AllI18n>,
  )
  return handlers
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('StorageHubPage', () => {
  it('renders the verdict, bar, trend, policy and list once each', () => {
    setup()
    renderHub()
    expect(
      screen.getByText(/38 GiB free of 100 GiB\./, { exact: false }),
    ).toBeInTheDocument()
    expect(screen.getByText('Healthy')).toBeInTheDocument()
    expect(screen.getByText(/Full in about 38 days/)).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Disk usage' }),
    ).toBeInTheDocument()
    expect(screen.getByText('trend')).toBeInTheDocument()
    expect(screen.getByText('policy')).toBeInTheDocument()
    expect(screen.getByText('list view=type filter=none')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Run in a terminal/ }),
    ).toBeInTheDocument()
  })

  it('clicking a bar category sets the filter and clicking it again clears it', async () => {
    setup()
    const first = renderHub()
    await userEvent.click(
      screen.getByRole('button', { name: 'Filter: Build cache' }),
    )
    expect(first.onFilterChange).toHaveBeenCalledWith('buildCache')
  })

  it('clears the filter when the active category is clicked again', async () => {
    setup()
    const h = renderHub({ filter: 'buildCache' })
    await userEvent.click(
      screen.getByRole('button', { name: 'Filter: Build cache' }),
    )
    expect(h.onFilterChange).toHaveBeenCalledWith(null)
    expect(
      screen.getByText('list view=type filter=buildCache'),
    ).toBeInTheDocument()
  })

  it('opens the drawer from the button and from the URL param', async () => {
    setup()
    const h = renderHub()
    await userEvent.click(
      screen.getByRole('button', { name: /Review cleanup/ }),
    )
    expect(h.onDrawerChange).toHaveBeenCalledWith(true)
  })

  it('shows the drawer when the drawer param is set', () => {
    setup()
    renderHub({ drawerOpen: true })
    expect(screen.getByText('drawer open')).toBeInTheDocument()
  })

  it('keeps Review cleanup disabled with the role tooltip for a viewer', async () => {
    setup({ role: 'viewer' })
    const h = renderHub()
    const button = screen.getByRole('button', { name: /Review cleanup/ })
    expect(button).toHaveAttribute('aria-disabled', 'true')
    await userEvent.click(button)
    expect(h.onDrawerChange).not.toHaveBeenCalled()
    await userEvent.hover(button)
    expect(await screen.findByText('Needs admin')).toBeInTheDocument()
  })

  it('keeps the rest of the page when disk usage fails', () => {
    setup({
      status: {
        data: undefined,
        isPending: false,
        isError: true,
        refetch: () => Promise.resolve(),
      },
    })
    renderHub()
    expect(
      screen.getByText('Disk usage could not be read right now.'),
    ).toBeInTheDocument()
    expect(screen.getByText('trend')).toBeInTheDocument()
    expect(screen.getByText('list view=type filter=none')).toBeInTheDocument()
  })

  it('passes the view param to the list', () => {
    setup()
    renderHub({ view: 'app' })
    expect(screen.getByText('list view=app filter=none')).toBeInTheDocument()
  })
})
