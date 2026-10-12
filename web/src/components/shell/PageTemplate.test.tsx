import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageTemplate } from './PageTemplate'

describe('PageTemplate hero', () => {
  it('orders tile, text block, then actions in one row', () => {
    render(
      <PageTemplate
        logo={<span data-testid="tile" />}
        eyebrow="Eyebrow"
        title="Title"
        description="Description"
        actions={<button type="button">Act</button>}
      />,
    )
    const hero = document.querySelector('[data-slot="page-hero"]')
    expect(hero).not.toBeNull()
    const kids = Array.from(hero?.children ?? [])
    expect(kids).toHaveLength(3)
    expect(kids[0]?.getAttribute('data-slot')).toBe('page-hero-tile')
    expect(kids[0]?.contains(screen.getByTestId('tile'))).toBe(true)
    expect(kids[1]?.querySelector('h1')?.textContent).toBe('Title')
    expect(kids[2]?.getAttribute('data-slot')).toBe('page-hero-actions')
  })
})
