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
