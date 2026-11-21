import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { MoreWays } from './MoreWays'
import { NewAppI18n } from '../../test/newAppI18n'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

function renderWays(defaultOpen: 'image' | null) {
  render(
    <NewAppI18n>
      <QueryClientProvider client={createTestQueryClient()}>
        <MoreWays onPlan={vi.fn()} embedded defaultOpen={defaultOpen} />
      </QueryClientProvider>
    </NewAppI18n>,
  )
}

describe('MoreWays', () => {
  it('shows the image form straight away when opened by default', () => {
    renderWays('image')
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument()
  })

  it('stays closed without a default', () => {
    renderWays(null)
    expect(screen.queryByRole('button', { name: 'Close' })).toBeNull()
  })
})
