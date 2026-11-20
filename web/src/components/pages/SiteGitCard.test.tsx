import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SiteGitCard from './SiteGitCard'

const h = vi.hoisted(() => ({ mutate: vi.fn() }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({
    data: {
      repo_url: 'https://github.com/a/site',
      branch: 'main',
      trigger_mode: 'push',
    },
  }),
}))
vi.mock('../../queries/gitDeploySettings', () => ({
  useSetGitDeploySettings: () => ({ mutate: h.mutate, isPending: false }),
}))
vi.mock('../../queries/gitSourceConnect', () => ({
  useDeployGitSource: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('../../queries/pagesSites', () => ({
  usePagesSiteDetail: () => ({ data: undefined }),
}))
vi.mock('../../queries/previewEnvironments', () => ({
  usePreviewEnvironments: () => ({ data: [] }),
}))

beforeEach(() => h.mutate.mockReset())

describe('SiteGitCard trigger', () => {
  it('shows the current mode and saves a change at once', () => {
    render(<SiteGitCard name="site" />)
    const group = screen.getByRole('radiogroup', { name: 'git.card.onPush' })
    expect(group.querySelector('input:checked')?.getAttribute('value')).toBe(
      'push',
    )
    fireEvent.click(screen.getByLabelText('git.card.triggerManual'))
    expect(h.mutate.mock.calls[0]?.[0]).toEqual({ trigger_mode: 'manual' })
  })
})
