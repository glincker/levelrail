import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { RecentBubbles, type Bubble } from './RecentBubbles'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const bubble = (label: string): Bubble => ({
  key: label,
  label,
  icon: <svg />,
  onPick: vi.fn(),
})

describe('RecentBubbles', () => {
  it('renders nothing without bubbles', () => {
    const { container } = render(<RecentBubbles bubbles={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('exposes each bubble as a named button with its Alt+number shortcut', () => {
    render(<RecentBubbles bubbles={[bubble('Apps'), bubble('Backups')]} />)
    expect(screen.getByRole('button', { name: 'Apps' })).toHaveAttribute(
      'aria-keyshortcuts',
      'Alt+1',
    )
    expect(screen.getByRole('button', { name: 'Backups' })).toHaveAttribute(
      'aria-keyshortcuts',
      'Alt+2',
    )
  })

  it('navigates when a bubble is clicked', async () => {
    const b = bubble('Apps')
    render(<RecentBubbles bubbles={[b]} />)
    await userEvent.click(screen.getByRole('button', { name: 'Apps' }))
    expect(b.onPick).toHaveBeenCalledOnce()
  })
})

describe('RecentBubbles shortcut labels', () => {
  const setPlatform = (value: string) =>
    vi.spyOn(window.navigator, 'platform', 'get').mockReturnValue(value)

  it('shows Option on Mac and Alt elsewhere in the tooltip', async () => {
    for (const [platform, label] of [
      ['MacIntel', '⌥1'],
      ['Win32', 'Alt+1'],
    ] as const) {
      const spy = setPlatform(platform)
      const { unmount } = render(<RecentBubbles bubbles={[bubble('Apps')]} />)
      await userEvent.hover(screen.getByRole('button', { name: 'Apps' }))
      expect(await screen.findByText(label)).toBeInTheDocument()
      unmount()
      spy.mockRestore()
    }
  })

  it('dims bubbles that do not match the query but keeps them clickable', () => {
    render(
      <RecentBubbles
        bubbles={[bubble('Apps'), bubble('Backups')]}
        query="back"
      />,
    )
    expect(screen.getByRole('button', { name: 'Apps' })).toHaveClass(
      'opacity-60',
    )
    expect(screen.getByRole('button', { name: 'Backups' })).not.toHaveClass(
      'opacity-60',
    )
  })
})
