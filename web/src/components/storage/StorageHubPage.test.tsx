import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import imagesEn from '../../locales/en/images.json'
import storageEn from '../../locales/en/storage.json'
import { jsonResponse } from '../../test/newSurfacesI18n'
import { StorageHubPage } from './StorageHubPage'
import type { StorageTab } from './storageTabs'

const nodes = vi.hoisted(() => ({ list: [] as { id: string; name: string }[] }))

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('../../queries/nodes', () => ({
  useNodeListOptional: () => ({ data: nodes.list }),
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
vi.mock('../images/DockerHealthCard', () => ({ DockerHealthCard: () => null }))
vi.mock('../images/SignaturePolicyCard', () => ({
  SignaturePolicyCard: () => null,
}))
vi.mock('../images/DockerfileLintCard', () => ({
  DockerfileLintCard: () => null,
}))
vi.mock('../builds/BuildCachePanel', () => ({
  BuildCachePanel: () => <p>cache panel</p>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'images', 'storage'],
  defaultNS: 'common',
  resources: {
    en: { common: commonEn, images: imagesEn, storage: storageEn },
  },
  interpolation: { escapeValue: false },
})

const GIB = 1024 ** 3
const inventory = {
  node: '',
  images: [],
  totals: {
    count: 0,
    size_bytes: 5 * GIB,
    reclaimable_bytes: 1 * GIB,
    protected_count: 0,
    unmanaged_unused_count: 0,
    unmanaged_unused_bytes: 1 * GIB,
    app_images: 0,
    other_images: 0,
  },
  retention: { keep_per_app: 3, min_age_hours: 24 },
  signatures: { mode: 'off', configured: false },
}
const status = {
  data_dir_total_bytes: 100 * GIB,
  data_dir_free_bytes: 38 * GIB,
  docker_connected: true,
  docker_disk_usage: {
    images_total_bytes: 9 * GIB,
    images_reclaimable_bytes: 0,
    containers_total_bytes: 2 * GIB,
    containers_reclaimable_bytes: 0,
    volumes_total_bytes: 3 * GIB,
    volumes_reclaimable_bytes: 0,
    build_cache_total_bytes: 4 * GIB,
    build_cache_reclaimable_bytes: 2 * GIB,
  },
}

function stub(opts: { remoteFails?: boolean; withUsage?: boolean } = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      if (url.startsWith('/api/v1/system/status')) {
        const body =
          opts.withUsage === false
            ? { ...status, docker_disk_usage: undefined }
            : status
        return Promise.resolve(jsonResponse(body))
      }
      if (url.startsWith('/api/v1/images?node=n1') && opts.remoteFails) {
        return Promise.resolve(jsonResponse({ error: 'unreachable' }, 502))
      }
      return Promise.resolve(jsonResponse(inventory))
    }),
  )
}

function renderHub(tab: StorageTab = 'images', onTabChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <StorageHubPage
          tab={tab}
          filter="all"
          onTabChange={onTabChange}
          onFilterChange={vi.fn()}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return onTabChange
}

afterEach(() => {
  vi.unstubAllGlobals()
  nodes.list = []
})

describe('StorageHubPage', () => {
  it('shows size and reclaimable per category from the data the page has', async () => {
    stub()
    renderHub()
    const bar = await screen.findByRole('region', { name: 'Disk breakdown' })
    expect(await within(bar).findByText('5.0 GiB')).toBeInTheDocument()
    expect(within(bar).getAllByText('2.0 GiB can be freed')).toHaveLength(2)
    expect(within(bar).getByText('3.0 GiB')).toBeInTheDocument()
    expect(within(bar).getByText('4.0 GiB')).toBeInTheDocument()
    expect(
      within(bar).getByText('38 GiB free of 100 GiB on the data disk'),
    ).toBeInTheDocument()
  })

  it('says not measured yet instead of inventing numbers', async () => {
    stub({ withUsage: false })
    renderHub()
    const bar = await screen.findByRole('region', { name: 'Disk breakdown' })
    expect(await within(bar).findAllByText('Not measured yet')).toHaveLength(3)
  })

  it('shows a calm unsupported state for a remote node that cannot report', async () => {
    nodes.list = [{ id: 'n1', name: 'edge-1' }]
    stub({ remoteFails: true })
    renderHub()
    await userEvent.selectOptions(await screen.findByLabelText('Node'), 'n1')
    expect(
      await screen.findByText('This node is not supported yet'),
    ).toBeInTheDocument()
    const bar = screen.getByRole('region', { name: 'Disk breakdown' })
    expect(within(bar).getAllByText('Not measured yet')).toHaveLength(4)
  })

  it('switches tabs through the URL handler and keeps engine details collapsed', async () => {
    stub()
    const onTabChange = renderHub()
    await userEvent.click(
      await screen.findByRole('tab', { name: 'Build cache' }),
    )
    expect(onTabChange).toHaveBeenCalledWith('cache')
    expect(screen.getByRole('tab', { name: 'Images' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(
      screen.getByRole('button', { name: 'Engine and signatures' }),
    ).toHaveAttribute('aria-expanded', 'false')
  })

  it('renders the build cache panel on the cache tab', async () => {
    stub()
    renderHub('cache')
    expect(await screen.findByText('cache panel')).toBeInTheDocument()
  })
})
