import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StaticResources } from './StaticResources'
import { StaticHealth } from './StaticHealth'

vi.mock('../../queries/pagesSites', () => ({
  usePagesSiteDetail: () => ({
    isPending: false,
    isError: false,
    data: {
      name: 's',
      domains: [],
      published: true,
      file_count: 12,
      size_bytes: 2048,
    },
  }),
  siteChecksQueryOptions: () => ({
    queryKey: ['checks'],
    queryFn: () => ({}),
  }),
}))
vi.mock('@tanstack/react-query', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({
    data: {
      score: 80,
      checks: [
        { id: 'a', status: 'fail', title: 'A', detail: '' },
        { id: 'b', status: 'warn', title: 'B', detail: '' },
      ],
    },
  }),
}))
vi.mock('../pages/SiteTrafficCard', () => ({
  SiteTrafficCard: () => <div>traffic card</div>,
}))
vi.mock('../pages/SiteChecksCard', () => ({
  SiteChecksCard: () => <div>checks card</div>,
}))

describe('static variants', () => {
  it('shows disk, file count and the traffic card, no container metrics', () => {
    render(<StaticResources name="s" />)
    expect(screen.getByText(/^2(\.0)? ?K/)).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('traffic card')).toBeInTheDocument()
    expect(screen.queryByText('CPU')).not.toBeInTheDocument()
  })

  it('shows the score, failing count and the checks card', () => {
    render(<StaticHealth name="s" />)
    expect(screen.getByText('80')).toBeInTheDocument()
    expect(screen.getByText('checks card')).toBeInTheDocument()
  })
})
