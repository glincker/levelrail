import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { appGroupKeys } from '../queries/appGroup'
import type { AppGroup } from '../types/appGroup'
import { LogsServiceSelect } from './LogsServiceSelect'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))

function group(names: string[]): AppGroup {
  return {
    services: names.map((name) => ({ name })),
  } as unknown as AppGroup
}

function renderWith(names: string[] | null) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  if (names) qc.setQueryData(appGroupKeys.detail('shop'), group(names))
  else vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('boom'))
  return render(
    <QueryClientProvider client={qc}>
      <LogsServiceSelect name="shop" />
    </QueryClientProvider>,
  )
}

describe('LogsServiceSelect', () => {
  beforeEach(() => {
    navigate.mockClear()
    vi.restoreAllMocks()
  })

  it('renders nothing for a single-service group', () => {
    const { container } = renderWith(['shop'])
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when the query errors', () => {
    const { container } = renderWith(null)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the current service for a multi-service group', () => {
    renderWith(['shop', 'shop-worker'])
    expect(screen.getByText('logs.serviceFilter.label')).toBeInTheDocument()
    expect(screen.getByRole('combobox')).toHaveTextContent('shop')
  })
})
