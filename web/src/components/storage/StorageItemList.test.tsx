import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import storageListEn from '../../locales/en/storageList.json'
import { jsonResponse } from '../../test/newSurfacesI18n'
import { StorageItemList } from './StorageItemList'

function mockApi(inventory: object) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string) =>
      Promise.resolve(
        input.includes('/system/storage/inventory')
          ? jsonResponse(inventory)
          : jsonResponse({ node: 'local', images: [] }),
      ),
    ),
  )
}

function renderList(
  props: Partial<Parameters<typeof StorageItemList>[0]> = {},
) {
  const i18n = i18next.createInstance()
  void i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { storageList: storageListEn } },
    interpolation: { escapeValue: false },
  })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onViewChange = vi.fn()
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <StorageItemList
          node="local"
          view="type"
          onViewChange={onViewChange}
          {...props}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return { onViewChange }
}

afterEach(() => vi.unstubAllGlobals())

describe('StorageItemList', () => {
  it('shows a calm panel and no table on an unsupported node', async () => {
    mockApi({
      node: 'n2',
      volumes: [],
      containers: [],
      unsupported: true,
      reason: 'Remote nodes are not listed yet',
    })
    renderList({ node: 'n2' })
    expect(
      await screen.findByText('Remote nodes are not listed yet'),
    ).toBeTruthy()
    expect(screen.queryByRole('list')).toBeNull()
  })

  it('lists a database volume as protected with its reason and switches view', async () => {
    mockApi({
      node: 'local',
      unsupported: false,
      containers: [],
      volumes: [
        {
          name: 'db-main',
          size_bytes: 2048,
          in_use: true,
          is_database: true,
          orphaned: false,
          protected: true,
          protect_reason: 'Holds the postgres data of shop',
          owner_app: 'shop',
        },
      ],
    })
    const { onViewChange } = renderList()
    expect(await screen.findByText('db-main')).toBeTruthy()
    expect(screen.getByText('Holds the postgres data of shop')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'By app' }))
    expect(onViewChange).toHaveBeenCalledWith('app')
  })
})
