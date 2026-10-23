import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CommonI18n } from '../../test/commonI18n'
import { CardSkeleton } from './card-skeleton'

describe('CardSkeleton', () => {
  it('exposes exactly one status with a translated label', () => {
    render(
      <CommonI18n>
        <CardSkeleton rows={3} />
      </CommonI18n>,
    )
    const statuses = screen.getAllByRole('status')
    expect(statuses).toHaveLength(1)
    expect(statuses[0]).toHaveTextContent('Loading')
  })

  it('hides the decorative card from assistive tech', () => {
    const { container } = render(
      <CommonI18n>
        <CardSkeleton rows={3} />
      </CommonI18n>,
    )
    const card = container.querySelector('[data-slot="card"]')
    expect(card).toHaveAttribute('aria-hidden', 'true')
    // Title bar plus three rows.
    expect(card?.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(4)
  })
})
