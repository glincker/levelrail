import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { ManagedByStackPanel } from './ManagedByStackPanel'
import {
  jsonResponse,
  renderWithProviders,
  stack,
} from '../stacks/stacksTestUtils'

const features = vi.hoisted(() => ({ list: ['iac'] as string[] }))
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => features.list,
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#x">{children}</a>,
}))

beforeEach(() => {
  features.list = ['iac']
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(jsonResponse(stack({ drift_policy: 'lock' })))),
  )
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ManagedByStackPanel', () => {
  it('shows revision, status, lock mode and the hint', async () => {
    renderWithProviders(<ManagedByStackPanel stack="shop" />)
    expect(await screen.findByText('Revision 3')).toBeInTheDocument()
    expect(screen.getByText('Applied')).toBeInTheDocument()
    expect(screen.getByText('Lock mode')).toBeInTheDocument()
    expect(screen.getByText(/Edit this in git/)).toBeInTheDocument()
  })

  it('renders nothing when unmanaged or iac is off', () => {
    const { container, rerender } = renderWithProviders(
      <ManagedByStackPanel stack={undefined} />,
    )
    expect(container).toBeEmptyDOMElement()
    features.list = []
    rerender(<ManagedByStackPanel stack="shop" />)
    expect(container).toBeEmptyDOMElement()
  })
})
