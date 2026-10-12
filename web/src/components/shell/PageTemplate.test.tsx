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

describe('PageTemplate aside slot', () => {
  const cases = [
    { name: 'default', width: undefined, expected: 'default' },
    { name: 'wide', width: 'wide' as const, expected: 'wide' },
    { name: 'full', width: 'full' as const, expected: 'full' },
  ]

  it.each(cases)(
    'renders the aside after main in DOM order ($name width)',
    ({ width, expected }) => {
      render(
        <PageTemplate
          title="Title"
          width={width}
          aside={<p>Rail</p>}
          asideLabel="Related"
        >
          <p>Main</p>
        </PageTemplate>,
      )
      const root = document.querySelector('[data-page-width]')
      expect(root?.getAttribute('data-page-width')).toBe(expected)
      const main = document.querySelector('[data-page-main]')
      const aside = screen.getByRole('complementary', { name: 'Related' })
      expect(aside.hasAttribute('data-page-aside')).toBe(true)
      expect(main?.contains(screen.getByText('Main'))).toBe(true)
      expect(aside.contains(screen.getByText('Rail'))).toBe(true)
      expect(
        main?.compareDocumentPosition(aside) ===
          Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBe(true)
    },
  )

  it('renders no aside landmark and no grid without the slot', () => {
    render(
      <PageTemplate title="Title">
        <p>Main</p>
      </PageTemplate>,
    )
    expect(document.querySelector('[data-page-aside]')).toBeNull()
    expect(document.querySelector('[data-page-body]')).toBeNull()
    expect(screen.queryByRole('complementary')).toBeNull()
  })
})
