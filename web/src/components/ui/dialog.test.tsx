import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent, DialogTitle } from './dialog'

describe('DialogContent material', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('opens with the glass material when reduced motion is requested', () => {
    vi.stubGlobal(
      'matchMedia',
      (query: string) =>
        ({
          matches: query.includes('prefers-reduced-motion'),
          media: query,
          addEventListener: vi.fn(),
          removeEventListener: vi.fn(),
        }) as unknown as MediaQueryList,
    )
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Rename</DialogTitle>
        </DialogContent>
      </Dialog>,
    )
    expect(screen.getByRole('dialog')).toHaveClass(
      'bg-popover',
      'motion-overlay',
    )
    expect(screen.getByText('Rename')).toBeInTheDocument()
  })
})
