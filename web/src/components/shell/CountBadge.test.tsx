import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CountBadge } from './CountBadge'

describe('CountBadge', () => {
  it('caps counts and keeps each badge in its own positioned button', () => {
    render(
      <div>
        <button type="button" className="relative" aria-label="12 new updates">
          <CountBadge count={12} tone="primary" />
        </button>
        <button type="button" className="relative" aria-label="150 unread">
          <CountBadge count={150} tone="destructive" />
        </button>
      </div>,
    )
    const a = screen.getByRole('button', { name: /12 new updates/ })
    const b = screen.getByRole('button', { name: /150 unread/ })
    expect(a).not.toBe(b)
    expect(a.textContent).toBe('9+')
    expect(b.textContent).toBe('99+')
    for (const btn of [a, b]) {
      const badge = btn.querySelector('[data-slot="count-badge"]')
      expect(btn.className).toContain('relative')
      expect(badge?.className).toContain('absolute')
      expect(badge?.className).toContain('min-w-4')
    }
  })
})
