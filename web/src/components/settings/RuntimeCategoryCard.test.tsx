import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { RuntimeCategoryCard } from './RuntimeCategoryCard'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: { count?: number }) => `${k}:${o?.count ?? ''}`,
  }),
}))

const base = {
  id: 'cat-security',
  title: 'Security',
  count: 4,
  weaker: 0,
  forceOpen: false,
  defaultOpen: false,
}

describe('RuntimeCategoryCard', () => {
  it('starts collapsed and toggles open', () => {
    render(
      <RuntimeCategoryCard {...base}>
        <p>row</p>
      </RuntimeCategoryCard>,
    )
    const trigger = screen.getByRole('button', { name: /Security/ })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
  })

  it('stays open while filtering and shows the weakened count', () => {
    render(
      <RuntimeCategoryCard {...base} weaker={2} forceOpen>
        <p>row</p>
      </RuntimeCategoryCard>,
    )
    expect(screen.getByRole('button', { name: /Security/ })).toHaveAttribute(
      'aria-expanded',
      'true',
    )
    expect(screen.getByText('category.weaker:2')).toBeInTheDocument()
  })
})
