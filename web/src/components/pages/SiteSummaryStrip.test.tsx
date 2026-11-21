import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SiteSummaryStrip } from './SiteSummaryStrip'

const h: { attempts: unknown[]; git: unknown; domains: string[] } = vi.hoisted(
  () => ({ attempts: [], git: undefined, domains: [] }),
)

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('../../hooks/useDeployProgress', () => ({
  useDeployProgress: () => ({ attempts: h.attempts, conditions: [] }),
}))
vi.mock('../../queries/apps', () => ({
  useApp: () => ({ data: { domains: h.domains } }),
}))
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({ data: h.git }),
}))
vi.mock('../../queries/pagesSites', () => ({
  usePagesSiteDetail: () => ({ data: undefined }),
}))

describe('SiteSummaryStrip', () => {
  it('shows the URL, live deploy message, repo chip and author', () => {
    h.domains = ['docs.example.com']
    h.git = { repo_url: 'https://github.com/acme/docs.git', branch: 'main' }
    h.attempts = [
      { id: 'a', status: 'succeeded', started_at: '2026-01-01T00:00:00Z' },
      {
        id: 'b',
        live: true,
        status: 'succeeded',
        commit_message: 'Fix nav\n\nbody',
        author: 'ana',
        started_at: '2026-01-01T00:00:00Z',
      },
    ]
    render(<SiteSummaryStrip name="docs" />)
    expect(
      screen.getAllByRole('link').map((a) => a.getAttribute('href')),
    ).toEqual(['https://docs.example.com', 'https://docs.example.com'])
    expect(screen.getByText('Fix nav')).toBeTruthy()
    expect(screen.getByText('acme/docs : main')).toBeTruthy()
    expect(screen.getByText('ana')).toBeTruthy()
  })

  it('stays calm with no domain, repo or deploy', () => {
    h.domains = []
    h.git = undefined
    h.attempts = []
    render(<SiteSummaryStrip name="docs" />)
    expect(screen.getByText('list.noUrl')).toBeTruthy()
    expect(screen.getByText('overview.strip.none')).toBeTruthy()
    expect(screen.queryByRole('link')).toBeNull()
  })
})
