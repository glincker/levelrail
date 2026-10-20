import { act, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Toaster, toast } from './toast'

describe('toast announcements', () => {
  it('announces a new toast through a live region', async () => {
    render(<Toaster />)
    act(() => {
      toast.add({ title: 'Saved the thing', type: 'success' })
    })
    expect(await screen.findByText('Saved the thing')).toBeInTheDocument()
    const live = document.querySelector('[aria-live]')
    expect(live).not.toBeNull()
  })
})
