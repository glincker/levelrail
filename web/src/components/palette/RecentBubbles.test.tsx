import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { RecentBubbles, type BubbleGroup } from './RecentBubbles'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const group = (over: Partial<BubbleGroup> = {}): BubbleGroup => ({
  id: 'recent',
  label: 'Recent',
  bubbles: [
    {
      key: 'a',
      label: 'Backups',
      icon: <svg />,
      onPick: vi.fn(),
      onRemove: vi.fn(),
      removeLabel: 'Remove Backups',
    },
  ],
  ...over,
})

describe('RecentBubbles', () => {
  it('renders nothing when every group is empty', () => {
    const { container } = render(
      <RecentBubbles groups={[group({ bubbles: [] })]} onClear={vi.fn()} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('picks a bubble and removes one without picking it', async () => {
    const g = group()
    render(<RecentBubbles groups={[g]} onClear={vi.fn()} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'Remove Backups' }),
    )
    expect(g.bubbles[0]?.onRemove).toHaveBeenCalledOnce()
    expect(g.bubbles[0]?.onPick).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Backups' }))
    expect(g.bubbles[0]?.onPick).toHaveBeenCalledOnce()
  })

  it('clears the history', async () => {
    const onClear = vi.fn()
    render(<RecentBubbles groups={[group()]} onClear={onClear} />)
    await userEvent.click(screen.getByRole('button', { name: 'bubbles.clear' }))
    expect(onClear).toHaveBeenCalledOnce()
  })
})
