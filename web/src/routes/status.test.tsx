import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StatusPage } from './status'
import type { AttentionItem } from '../lib/attention'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const useAttentionItems = vi.fn()
vi.mock('../queries/attention', () => ({
  useAttentionItems: () => useAttentionItems() as unknown,
}))

const mutate = vi.fn()
vi.mock('../queries/apps', () => ({
  useRestartApp: () => ({ mutate, isPending: false }),
}))

const items: AttentionItem[] = [
  {
    id: 'a',
    severity: 'critical',
    title: 'web is crashing',
    detail: 'restarted 5 times',
    target: { kind: 'app', name: 'web' },
  },
  {
    id: 'n',
    severity: 'warning',
    title: 'node offline',
    detail: 'last seen 1h ago',
    target: { kind: 'node', id: 'n1' },
  },
]

describe('StatusPage', () => {
  afterEach(() => {
    cleanup()
    useAttentionItems.mockReset()
    mutate.mockReset()
  })

  it('shows a loading status', () => {
    useAttentionItems.mockReturnValue({ items: [], isLoading: true })
    render(<StatusPage />)
    expect(
      screen.getByRole('status', { name: 'Loading status' }),
    ).toBeInTheDocument()
  })

  it('shows the healthy empty state', () => {
    useAttentionItems.mockReturnValue({ items: [], isLoading: false })
    render(<StatusPage />)
    expect(screen.getByText('All systems healthy')).toBeVisible()
  })

  it('renders items with severity and uniquely named actions', async () => {
    useAttentionItems.mockReturnValue({ items, isLoading: false })
    render(<StatusPage />)
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('critical')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'View logs for web' }),
    ).toBeVisible()
    expect(screen.getByRole('link', { name: 'Open node n1' })).toBeVisible()
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: 'Restart web' }))
    expect(mutate).toHaveBeenCalledWith('web', expect.any(Object))
  })
})
