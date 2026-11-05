import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StatusPage } from './StatusPage'
import type { AttentionItem } from '@/lib/attention'

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

const silenceRows = vi.fn(() => [] as unknown[])
const useAttentionItems = vi.fn()
vi.mock('@/queries/attention', () => ({
  useAttentionItems: () => useAttentionItems() as unknown,
}))

vi.mock('@/queries/silences', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/queries/silences')>()),
  useSilences: () => ({ data: silenceRows() as unknown }),
  useCreateSilence: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteSilence: () => ({ mutate: vi.fn(), isPending: false }),
}))

// Both sections fetch their own data and have their own tests.
vi.mock('@/components/monitors/MonitorsSection', () => ({
  MonitorsSection: () => null,
}))
vi.mock('@/components/incidents/IncidentsSection', () => ({
  IncidentsSection: () => null,
}))

const mutate = vi.fn()
vi.mock('@/queries/apps', () => ({
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
    silenceRows.mockReset()
    silenceRows.mockReturnValue([])
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
    expect(
      screen.getByText('attention:statusPage.healthyDescription'),
    ).toBeVisible()
  })

  it('keeps recommendations out of the main list in a collapsed advisory block', async () => {
    const advice: AttentionItem[] = [
      {
        id: 'doctor:Backups',
        severity: 'warning',
        title: 'Backups are off',
        detail: 'enable them',
        target: { kind: 'system' },
        class: 'recommendation',
      },
      {
        id: 'update',
        severity: 'warning',
        title: 'New version',
        detail: 'available',
        target: { kind: 'system' },
        class: 'recommendation',
      },
    ]
    useAttentionItems.mockReturnValue({
      items: [...items, ...advice],
      isLoading: false,
    })
    render(<StatusPage />)
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    const toggle = screen.getByRole('button', {
      name: 'statusPage.advisory.title',
    })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Backups are off')).not.toBeVisible()
    await userEvent.setup().click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Backups are off')).toBeVisible()
  })

  it('shows a calm line when only advice remains', () => {
    useAttentionItems.mockReturnValue({
      items: [
        {
          id: 'update',
          severity: 'warning',
          title: 'New version',
          detail: 'available',
          target: { kind: 'system' },
          class: 'recommendation',
        },
      ],
      isLoading: false,
    })
    render(<StatusPage />)
    expect(screen.getByText('statusPage.nothingNeedsAttention')).toBeVisible()
    expect(
      screen.getByRole('button', {
        name: 'statusPage.advisory.title',
      }),
    ).toBeVisible()
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

  it.each(['doctor:code_a', 'doctor:Alpha'])(
    'hides a doctor row silenced under %s',
    (key) => {
      silenceRows.mockReturnValue([
        {
          id: 's1',
          issue_key: key,
          created_by: 'me',
          created_at: '2026-01-01T00:00:00Z',
          expired: false,
        },
      ])
      useAttentionItems.mockReturnValue({
        items: [
          {
            id: 'doctor:Alpha',
            legacyIds: ['doctor:code_a'],
            severity: 'warning',
            title: 'Alpha check',
            detail: 'd',
            target: { kind: 'system' },
          },
        ],
        isLoading: false,
      })
      render(<StatusPage />)
      expect(screen.queryByRole('link', { name: /Alpha/ })).toBeNull()
      expect(screen.getByText('silence.sectionTitle')).toBeInTheDocument()
    },
  )
})
