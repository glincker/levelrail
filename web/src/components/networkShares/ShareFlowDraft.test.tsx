import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { ShareFlow } from './ShareFlow'
import { Providers } from './testProviders'

function mount() {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response('[]', {
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    ),
  )
  return render(
    <Providers>
      <Dialog open>
        <DialogContent>
          <ShareFlow onClose={() => undefined} />
        </DialogContent>
      </Dialog>
    </Providers>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ShareFlow draft', () => {
  it('offers to continue after the dialog is closed and reopened', () => {
    const first = mount()
    fireEvent.change(screen.getByLabelText('Host'), {
      target: { value: 'nas.lan' },
    })
    first.unmount()

    mount()
    expect(screen.getByText(/You have an unsaved draft/)).toBeTruthy()
    fireEvent.click(
      screen.getByRole('button', { name: 'Continue where you left off' }),
    )
    expect(screen.getByLabelText<HTMLInputElement>('Host').value).toBe(
      'nas.lan',
    )
    expect(screen.queryByText(/You have an unsaved draft/)).toBeNull()
  })
})
