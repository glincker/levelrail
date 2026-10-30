import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AttentionWidget } from './AttentionWidget'
import { renderWidget } from './testKit'

vi.mock('@tanstack/react-router', async () =>
  (await import('./testKit')).routerMock(),
)
const state = vi.hoisted(() => ({
  r: {} as { items: unknown[]; isLoading: boolean },
}))
vi.mock('@/queries/attention', () => ({ useAttentionItems: () => state.r }))

const item = (n: number) => ({
  id: `i${n}`,
  severity: 'critical',
  title: `Issue ${n}`,
  detail: '',
  target: { kind: 'system' },
})

describe('AttentionWidget', () => {
  it('shows all clear when empty', () => {
    state.r = { items: [], isLoading: false }
    renderWidget(<AttentionWidget />)
    expect(
      screen.getByText('All clear. Nothing needs attention.'),
    ).toBeInTheDocument()
  })

  it('caps at four items and links to Status', () => {
    state.r = { items: [1, 2, 3, 4, 5, 6].map(item), isLoading: false }
    renderWidget(<AttentionWidget />)
    expect(screen.getByText('Issue 4')).toBeInTheDocument()
    expect(screen.queryByText('Issue 5')).toBeNull()
    expect(screen.getByRole('link')).toHaveAttribute('href', '/status')
    expect(screen.getByText('2 more on Status')).toBeInTheDocument()
  })
})
