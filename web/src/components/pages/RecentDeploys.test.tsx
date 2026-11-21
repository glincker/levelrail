import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { RecentDeploys } from './RecentDeploys'

const h: { attempts: unknown[]; navigate: ReturnType<typeof vi.fn> } =
  vi.hoisted(() => ({ attempts: [], navigate: vi.fn() }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o ? `${k}:${JSON.stringify(o)}` : k,
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => h.navigate,
}))
vi.mock('../../hooks/useDeployProgress', () => ({
  useDeployProgress: () => ({ attempts: h.attempts, conditions: [] }),
}))
vi.mock('./DeployRowActions', () => ({
  default: () => <span>actions</span>,
}))

const attempt = (i: number) => ({
  id: `dep_${i}`,
  status: 'succeeded',
  commit_sha: `abcdef${i}0123`,
  commit_message: `msg ${i}`,
  branch: 'main',
  author: 'ana',
  started_at: '2026-01-01T00:00:00Z',
  finished_at: '2026-01-01T00:03:03Z',
})

describe('RecentDeploys', () => {
  it('renders at most five rows with sha, message, state and build time', () => {
    h.attempts = Array.from({ length: 7 }, (_, i) => attempt(i))
    render(<RecentDeploys name="docs" onViewAll={vi.fn()} />)
    expect(screen.getAllByTestId('deploy-row')).toHaveLength(5)
    expect(screen.getByText('abcdef0')).toBeTruthy()
    expect(screen.getByText('msg 0')).toBeTruthy()
    expect(screen.getAllByText('overview.state.succeeded').length).toBe(5)
    expect(
      screen.getAllByText('overview.duration:{"minutes":3,"seconds":3}').length,
    ).toBe(5)
  })

  it('opens the deploy on a row click but not on a button click', () => {
    h.attempts = [attempt(1)]
    h.navigate.mockReset()
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
    render(<RecentDeploys name="docs" onViewAll={vi.fn()} />)
    fireEvent.click(screen.getByLabelText('recent.copySha'))
    expect(h.navigate).not.toHaveBeenCalled()
    fireEvent.click(screen.getByText('msg 1'))
    expect(h.navigate).toHaveBeenCalledWith({
      to: '/apps/$name/deploys/$deployId/logs',
      params: { name: 'docs', deployId: 'dep_1' },
    })
  })

  it('shows an empty line without a table', () => {
    h.attempts = []
    render(<RecentDeploys name="docs" onViewAll={vi.fn()} />)
    expect(screen.getByText('recent.none')).toBeTruthy()
    expect(screen.queryByRole('table')).toBeNull()
  })
})
