import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { AppStatusStrip } from './AppStatusStrip'

const reach = vi.hoisted(() => ({
  value: {
    reach: { state: 'reachable', reason: 'ok' },
    shareUrl: 'https://web.example.com',
  },
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="#x">{children}</a>
  ),
}))
vi.mock('@/components/kit', () => ({ RelativeTime: () => <span>ago</span> }))
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({ data: undefined }),
}))
vi.mock('../../queries/healthScore', () => ({
  useAppHealthScore: () => ({
    data: {
      categories: [{ key: 'a', label: 'A', status: 'warn', reason: 'r' }],
    },
  }),
}))
vi.mock('./useUrlReachability', () => ({
  useUrlReachability: () => reach.value,
}))
vi.mock('./AutoUrlInfo', () => ({ AutoUrlInfo: () => null }))
vi.mock('./TagChips', () => ({ TagChips: () => null }))
vi.mock('../apps/AppHeaderBadges', () => ({ AppHeaderBadges: () => null }))

const app = { name: 'web' } as AppDetail
const attempts = [
  {
    id: 'd1',
    service_name: 'web',
    image: 'i',
    status: 'succeeded',
    started_at: '2026-01-01T00:00:00Z',
  },
] as never

function strip() {
  return render(
    <AppStatusStrip
      app={app}
      attempts={attempts}
      url="https://web.example.com"
      attentionCount={0}
    />,
  )
}

describe('AppStatusStrip', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('shows deploy, warnings and the health suggestion', () => {
    strip()
    expect(screen.getByText('state.succeeded')).toBeInTheDocument()
    expect(screen.getByText('status.warnings.label')).toBeInTheDocument()
    expect(screen.getByText('status.health.add')).toBeInTheDocument()
  })

  it('remembers a dismissed health suggestion per app', () => {
    const { unmount } = strip()
    fireEvent.click(screen.getByLabelText('status.health.dismiss'))
    expect(screen.queryByText('status.health.add')).toBeNull()
    unmount()
    strip()
    expect(screen.queryByText('status.health.add')).toBeNull()
    expect(screen.getByText('status.health.none')).toBeInTheDocument()
  })

  it('offers the QR icon only when an address can be shared', () => {
    const { unmount } = strip()
    expect(screen.getByLabelText('status.qr.open')).toBeInTheDocument()
    unmount()
    reach.value = {
      reach: { state: 'local_only', reason: 'local_host' },
      shareUrl: null,
    }
    strip()
    expect(screen.queryByLabelText('status.qr.open')).toBeNull()
    expect(screen.getByText('status.chip.local_only')).toBeInTheDocument()
  })
})
