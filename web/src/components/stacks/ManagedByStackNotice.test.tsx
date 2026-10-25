import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { ManagedByStackBanner, ManagedByStackTag } from './ManagedByStackNotice'
import { jsonResponse, renderWithProviders, stack } from './stacksTestUtils'

const features = vi.hoisted(() => ({ list: ['iac'] as string[] }))
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => features.list,
}))

// The real Link needs a router; only the target matters here.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
  }: {
    to: string
    search: { stack: string }
    children: ReactNode
  }) => <a href={`${to}?stack=${search.stack}`}>{children}</a>,
}))

beforeEach(() => {
  features.list = ['iac']
})
afterEach(() => {
  vi.unstubAllGlobals()
})

function stubStack(policy: 'warn' | 'lock') {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(jsonResponse(stack({ drift_policy: policy })))),
  )
}

describe('ManagedByStackTag', () => {
  it('shows the badge when iac is on and a stack owns the resource', () => {
    renderWithProviders(<ManagedByStackTag stack="shop" />)
    expect(screen.getByText('Stack shop')).toBeInTheDocument()
  })

  it('renders nothing without a stack or with iac off', () => {
    const { container, rerender } = renderWithProviders(
      <ManagedByStackTag stack={undefined} />,
    )
    expect(container).toBeEmptyDOMElement()
    features.list = []
    rerender(<ManagedByStackTag stack="shop" />)
    expect(screen.queryByText('Stack shop')).not.toBeInTheDocument()
  })
})

describe('ManagedByStackBanner', () => {
  it('warns about drift for a warn stack', async () => {
    stubStack('warn')
    renderWithProviders(<ManagedByStackBanner stack="shop" />)
    expect(
      await screen.findByText(
        'Managed by stack shop. Manual changes may be reverted or reported as drift.',
      ),
    ).toBeInTheDocument()
  })

  it('says the stack is locked for a lock stack', async () => {
    stubStack('lock')
    renderWithProviders(<ManagedByStackBanner stack="shop" />)
    expect(
      await screen.findByText(/which locks its resources/),
    ).toBeInTheDocument()
  })

  it('renders nothing when iac is off', () => {
    features.list = []
    const { container } = renderWithProviders(
      <ManagedByStackBanner stack="shop" />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
