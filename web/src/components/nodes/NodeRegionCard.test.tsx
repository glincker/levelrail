import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import type { NodeResource } from '../../types/nodeDetail'
import { NodeRegionCard } from './NodeRegionCard'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

function node(over: Partial<NodeResource> = {}): NodeResource {
  return {
    id: 'nd_1',
    name: 'web-1',
    region: 'eu',
    provider: 'hetzner',
    zone: 'fsn1',
    ...over,
  } as NodeResource
}

function mockFetch(body: unknown) {
  const fetchMock = vi.fn<typeof fetch>(() =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(body),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderCard(n: NodeResource) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <NodeRegionCard node={n} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('NodeRegionCard', () => {
  it('shows stored labels and keeps Save disabled until edited', () => {
    renderCard(node())
    expect(screen.getByPlaceholderText('e.g. eu-central')).toHaveValue('eu')
    expect(screen.getByPlaceholderText('e.g. hetzner')).toHaveValue('hetzner')
    expect(screen.getByPlaceholderText('e.g. fsn1')).toHaveValue('fsn1')
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })

  it('shows empty inputs for an unlabeled node', () => {
    renderCard(
      node({ region: undefined, provider: undefined, zone: undefined }),
    )
    expect(screen.getByPlaceholderText('e.g. fsn1')).toHaveValue('')
  })

  it('sends trimmed labels for all three fields on save', async () => {
    const fetchMock = mockFetch(node({ zone: 'nbg1' }))
    renderCard(node())
    fireEvent.change(screen.getByPlaceholderText('e.g. fsn1'), {
      target: { value: ' nbg1 ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(url).toBe('/api/v1/nodes/nd_1/region')
    expect(init?.method).toBe('PUT')
    expect(JSON.parse(init?.body as string)).toEqual({
      region: 'eu',
      provider: 'hetzner',
      zone: 'nbg1',
    })
  })
})
