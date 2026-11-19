import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SiteHero } from './SiteHero'
import { siteState } from './pageHelpers'

const detail: { current: unknown } = vi.hoisted(() => ({
  current: undefined,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('./SiteSuccess', () => ({ SiteUrlChip: () => <span>chip</span> }))
vi.mock('../../queries/pagesSites', () => ({
  usePagesSiteDetail: () => ({ data: detail.current }),
}))

const base = {
  name: 'docs',
  domains: [],
  published: true,
  file_count: 3,
  size_bytes: 2048,
}

describe('SiteHero', () => {
  it('shows the live state, size and the publish action', () => {
    detail.current = base
    const onPublish = vi.fn()
    render(<SiteHero name="docs" onPublish={onPublish} onDelete={vi.fn()} />)
    expect(screen.getByText('hero.status.live')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /detail.publish/ }))
    expect(onPublish).toHaveBeenCalled()
  })

  it('derives publishing and not published from the detail only', () => {
    expect(siteState(undefined)).toBeNull()
    expect(siteState({ ...base, published: false })).toBe('notPublished')
    expect(
      siteState({
        ...base,
        latest_deploy: {
          id: '1',
          status: 'running',
          source: 'drop',
          started_at: '2026-01-01T00:00:00Z',
        },
      }),
    ).toBe('publishing')
  })
})
