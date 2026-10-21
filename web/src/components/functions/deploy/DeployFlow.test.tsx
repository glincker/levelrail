import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { NewSurfacesProviders } from '../../../test/NewSurfacesProviders'
import { jsonResponse } from '../../../test/newSurfacesI18n'
import { DeployFlow } from './DeployFlow'

function renderFlow() {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(jsonResponse([]))),
  )
  render(
    <NewSurfacesProviders>
      <Dialog open>
        <DialogContent>
          <DeployFlow onClose={() => undefined} />
        </DialogContent>
      </Dialog>
    </NewSurfacesProviders>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DeployFlow', () => {
  it('shows templates and own-code cards on the first step', () => {
    renderFlow()
    expect(screen.getByText('Webhook receiver')).toBeTruthy()
    expect(screen.getByText('Use a container image')).toBeTruthy()
  })

  it('shows its own message instead of the browser bubble', () => {
    renderFlow()
    fireEvent.click(screen.getByText('Use a container image'))
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(screen.getByRole('alert').textContent).toBe(
      'Enter the image to run.',
    )
  })

  it('suggests a name from the image and reaches the summary', () => {
    renderFlow()
    fireEvent.click(screen.getByText('Use a container image'))
    fireEvent.change(screen.getByLabelText('Image'), {
      target: { value: 'ghcr.io/you/Hello:1.0' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(screen.getByLabelText<HTMLInputElement>('Name').value).toBe('hello')
    expect(
      screen.getByText(/run the image as is, sleep after 10 minutes/),
    ).toBeTruthy()
  })
})
