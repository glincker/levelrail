import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BetaBadge } from './BetaBadge'

describe('BetaBadge', () => {
  it('renders a labelled badge that carries its hint as the title', () => {
    render(<BetaBadge />)
    const badge = screen.getByTestId('beta-badge')
    expect(badge).toBeInTheDocument()
    expect(badge.textContent?.length).toBeGreaterThan(0)
    expect(badge.getAttribute('title')?.length).toBeGreaterThan(0)
  })
})
