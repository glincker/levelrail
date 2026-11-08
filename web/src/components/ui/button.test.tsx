import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Button } from './button'

describe('Button loading', () => {
  it('keeps the label, sets aria-busy and swallows clicks', () => {
    const onClick = vi.fn()
    render(
      <Button loading onClick={onClick}>
        Save
      </Button>,
    )
    const btn = screen.getByRole('button')
    expect(btn.getAttribute('aria-busy')).toBe('true')
    expect(btn.textContent).toContain('Save')
    fireEvent.click(btn)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('renders the accent primary by default', () => {
    render(<Button>Go</Button>)
    expect(screen.getByRole('button').className).toContain(
      'bg-(--glinr-accent-solid)',
    )
  })
})
