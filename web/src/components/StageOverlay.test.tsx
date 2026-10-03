import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StageOverlay } from './StageOverlay'
import { mockReducedMotion } from './kit/testUtils'

let experimentalOn: string[] = []
const navigate = vi.fn()

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))
vi.mock('@/hooks/useExperimental', () => ({
  useExperimentalFeatures: () => experimentalOn,
}))

beforeEach(() => {
  experimentalOn = []
  mockReducedMotion(false)
  navigate.mockClear()
})

describe('StageOverlay', () => {
  it('renders nothing when closed', () => {
    render(<StageOverlay open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('shows a tile for every non-gated GO_TARGETS entry', () => {
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    expect(
      screen.getByRole('dialog', { name: 'Quick navigation' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Go to Apps' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Go to Nodes' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Go to Settings' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Go to Load balancers' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Go to AI models' }),
    ).not.toBeInTheDocument()
  })

  it('shows gated tiles once their experimental feature is on', () => {
    experimentalOn = ['load-balancer', 'ai-models']
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    expect(
      screen.getByRole('button', { name: 'Go to Load balancers' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Go to AI models' }),
    ).toBeInTheDocument()
  })

  it('navigates and closes when a tile is selected', async () => {
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    render(<StageOverlay open onOpenChange={onOpenChange} />)
    await user.click(screen.getByRole('button', { name: 'Go to Apps' }))
    expect(navigate).toHaveBeenCalledWith({ to: '/apps' })
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('moves the roving tab stop with arrow keys', () => {
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    const apps = screen.getByRole('button', { name: 'Go to Apps' })
    const nodes = screen.getByRole('button', { name: 'Go to Nodes' })
    apps.focus()
    expect(apps).toHaveFocus()
    fireEvent.keyDown(apps, { key: 'ArrowRight' })
    expect(nodes).toHaveFocus()
    expect(nodes).toHaveAttribute('tabindex', '0')
    expect(apps).toHaveAttribute('tabindex', '-1')
  })

  it('wraps from the last tile back to the first with ArrowRight', () => {
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    const apps = screen.getByRole('button', { name: 'Go to Apps' })
    const settings = screen.getByRole('button', { name: 'Go to Settings' })
    settings.focus()
    fireEvent.keyDown(settings, { key: 'ArrowRight' })
    expect(apps).toHaveFocus()
  })

  it('jumps to the last tile with End', () => {
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    const apps = screen.getByRole('button', { name: 'Go to Apps' })
    const settings = screen.getByRole('button', { name: 'Go to Settings' })
    apps.focus()
    fireEvent.keyDown(apps, { key: 'End' })
    expect(settings).toHaveFocus()
  })

  it('shows the small reference strip for the remaining shortcuts', () => {
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    expect(screen.getByText('Open the command palette')).toBeInTheDocument()
    expect(screen.getByText('Open quick navigation')).toBeInTheDocument()
    expect(
      screen.getByText('Close a dialog, or leave the search field'),
    ).toBeInTheDocument()
  })

  it('skips the enter/exit animation when reduced motion is preferred', () => {
    mockReducedMotion(true)
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    const popup = document.querySelector('[data-slot="stage-overlay-popup"]')
    const backdrop = document.querySelector(
      '[data-slot="stage-overlay-backdrop"]',
    )
    expect(popup?.className).not.toContain('animate-in')
    expect(backdrop?.className).not.toContain('animate-in')
  })

  it('applies the enter/exit animation by default', () => {
    render(<StageOverlay open onOpenChange={vi.fn()} />)
    const popup = document.querySelector('[data-slot="stage-overlay-popup"]')
    expect(popup?.className).toContain('animate-in')
  })
})
