import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DeployRowActions from './DeployRowActions'

const h = vi.hoisted(() => ({
  mutate: vi.fn(),
  data: undefined as string | undefined,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({
    data: [
      {
        id: 'dep_a',
        status: 'succeeded',
        url: 'https://ab12cd34--site.localhost',
        live: false,
        promotable: true,
      },
      { id: 'dep_b', status: 'succeeded', live: true, promotable: false },
    ],
  }),
}))
vi.mock('../../queries/pagesDeploys', () => ({
  pagesDeploysQueryOptions: () => ({}),
  usePromoteDeploy: () => ({
    mutate: h.mutate,
    isPending: false,
    error: null,
    data: h.data,
  }),
}))

beforeEach(() => {
  h.mutate.mockReset()
  h.data = undefined
})

describe('DeployRowActions', () => {
  it('links to the deploy URL and promotes after confirming', () => {
    render(<DeployRowActions name="site" deployId="dep_a" />)
    expect(screen.getByLabelText('deployUrl.open').getAttribute('href')).toBe(
      'https://ab12cd34--site.localhost',
    )
    fireEvent.click(screen.getByText('promote.action'))
    fireEvent.click(screen.getAllByText('promote.action').at(-1)!)
    expect(h.mutate.mock.calls[0]?.[0]).toBe('dep_a')
  })

  it('offers nothing for the live deploy without a URL', () => {
    render(<DeployRowActions name="site" deployId="dep_b" />)
    expect(screen.queryByText('promote.action')).toBeNull()
    expect(screen.queryByLabelText('deployUrl.open')).toBeNull()
  })

  it('shows the going-live chip only while serving is pending', () => {
    const { unmount } = render(
      <DeployRowActions name="site" deployId="dep_a" />,
    )
    expect(screen.queryByText('promote.pending')).toBeNull()
    unmount()
    h.data = 'pending'
    render(<DeployRowActions name="site" deployId="dep_a" />)
    expect(screen.getByText('promote.pending')).toBeTruthy()
  })
})
