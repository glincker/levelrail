import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Label } from './label'
import { Separator } from './separator'

describe('glinui adapters', () => {
  it('Separator keeps role and orientation', () => {
    render(<Separator orientation="vertical" />)
    const sep = screen.getByRole('separator')
    expect(sep).toHaveAttribute('aria-orientation', 'vertical')
    expect(sep).toHaveAttribute('data-slot', 'separator')
  })

  it('Label labels its control', () => {
    render(
      <>
        <Label htmlFor="n">Name</Label>
        <input id="n" />
      </>,
    )
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
  })
})
