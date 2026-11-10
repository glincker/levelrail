import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import type { NodeThresholds } from '../../queries/nodeThresholds'
import { NodeThresholdsCard } from './NodeThresholdsCard'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

const fixture: NodeThresholds = {
  node_id: 'nd_1',
  defaults: { cpu_percent: 85, memory_percent: 90, disk_percent: 90 },
  override: { cpu_percent: 95, memory_percent: null, disk_percent: null },
  effective: { cpu_percent: 95, memory_percent: 90, disk_percent: 90 },
  sustained_intervals: 3,
}

function mockFetch(status: number, body: unknown) {
  const fetchMock = vi.fn<typeof fetch>(() =>
    Promise.resolve({
      ok: status < 400,
      status,
      json: () => Promise.resolve(body),
      text: () => Promise.resolve(JSON.stringify(body)),
      headers: new Headers({ 'content-type': 'application/json' }),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <NodeThresholdsCard nodeId="nd_1" />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('NodeThresholdsCard', () => {
  it('prefills the override and shows defaults as placeholders', async () => {
    mockFetch(200, fixture)
    renderCard()
    expect(await screen.findByLabelText('CPU %')).toHaveValue('95')
    expect(screen.getByLabelText('Memory %')).toHaveAttribute(
      'placeholder',
      'Default 90',
    )
  })

  it('blocks out-of-range values before saving', async () => {
    mockFetch(200, fixture)
    renderCard()
    fireEvent.change(await screen.findByLabelText('Disk %'), {
      target: { value: '150' },
    })
    expect(
      screen.getByRole('button', { name: 'Save thresholds' }),
    ).toBeDisabled()
  })

  it('shows the error state', async () => {
    mockFetch(500, { error: 'boom' })
    renderCard()
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
  })
})
