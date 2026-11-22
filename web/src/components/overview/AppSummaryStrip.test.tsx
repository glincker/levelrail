import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { AppSummaryStrip } from './AppSummaryStrip'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@/components/kit', () => ({
  RelativeTime: () => <span>ago</span>,
}))
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({
    data: { repo_url: 'https://github.com/acme/web.git', branch: 'main' },
  }),
}))
vi.mock('./UrlLine', () => ({ UrlLine: () => <span>url</span> }))
vi.mock('./AutoUrlInfo', () => ({ AutoUrlInfo: () => null }))
vi.mock('./TagChips', () => ({ TagChips: () => null }))
vi.mock('../apps/AppHeaderBadges', () => ({ AppHeaderBadges: () => null }))

const app = { name: 'web' } as AppDetail

describe('AppSummaryStrip', () => {
  it('shows message, state, repo chip and author', () => {
    render(
      <AppSummaryStrip
        app={app}
        url="https://web.example.com"
        attentionCount={0}
        attempts={[
          {
            id: 'd1',
            service_name: 'web',
            image: 'i',
            status: 'failed',
            started_at: '2026-01-01T00:00:00Z',
            commit_message: 'fix it\nmore',
            author: 'ada',
          },
        ]}
      />,
    )
    expect(screen.getByText('fix it')).toBeInTheDocument()
    expect(screen.getByText('state.failed')).toBeInTheDocument()
    expect(screen.getByText(/acme\/web : main/)).toBeInTheDocument()
    expect(screen.getByText('ada')).toBeInTheDocument()
  })

  it('says nothing was deployed yet', () => {
    render(
      <AppSummaryStrip app={app} url={null} attentionCount={0} attempts={[]} />,
    )
    expect(screen.getByText('summary.none')).toBeInTheDocument()
  })
})
