import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ActionMenu } from './ActionMenu'
import { EmptyState } from './EmptyState'
import { InfoTip } from './InfoTip'
import { MetricTile } from './MetricTile'
import { Suggestion } from './Suggestion'
import { SuggestionList } from './SuggestionList'
import { Timeline } from './Timeline'
import { mockReducedMotion } from './testUtils'

afterEach(() => {
  vi.useRealTimers()
  mockReducedMotion(false)
})

describe('MetricTile', () => {
  it('renders label, value and unit', () => {
    render(<MetricTile label="Requests" value="120" unit="/min" />)
    expect(screen.getByText('Requests')).toBeInTheDocument()
    expect(screen.getByText('120')).toBeInTheDocument()
    expect(screen.getByText('/min')).toBeInTheDocument()
  })
  it.each([
    ['up', 'up', 'success'],
    ['up', 'down', 'danger'],
    ['down', 'down', 'success'],
    ['down', 'up', 'danger'],
  ] as const)('delta %s when good is %s is %s', (direction, goodWhen, tone) => {
    render(
      <MetricTile
        label="m"
        value={1}
        delta={{ value: 4, direction, goodWhen }}
      />,
    )
    expect(screen.getByTestId('metric-delta')).toHaveAttribute(
      'data-tone',
      tone,
    )
  })
  it('treats a zero delta as neutral', () => {
    render(
      <MetricTile
        label="m"
        value={1}
        delta={{ value: 0, direction: 'up', goodWhen: 'up' }}
      />,
    )
    expect(screen.getByTestId('metric-delta')).toHaveAttribute(
      'data-tone',
      'neutral',
    )
  })
  it('embeds a sparkline when a series is given', () => {
    render(<MetricTile label="CPU" value={3} series={[1, 2, 3]} />)
    expect(screen.getByRole('img', { name: 'CPU trend' })).toBeInTheDocument()
  })
  it('shows a skeleton while loading', () => {
    render(<MetricTile label="CPU" value={3} loading />)
    expect(screen.getByTestId('skeleton-tile')).toBeInTheDocument()
    expect(screen.queryByText('CPU')).toBeNull()
  })
  it('is a keyboard operable button when clickable', async () => {
    const onClick = vi.fn()
    render(<MetricTile label="CPU" value="3" onClick={onClick} />)
    await userEvent.tab()
    expect(screen.getByRole('button')).toHaveFocus()
    await userEvent.keyboard('{Enter}')
    expect(onClick).toHaveBeenCalledTimes(1)
  })
  it('is not a button when not clickable', () => {
    render(<MetricTile label="CPU" value="3" />)
    expect(screen.queryByRole('button')).toBeNull()
  })
})

describe('InfoTip', () => {
  it('opens from the keyboard, sets aria-describedby and closes on Escape', async () => {
    render(<InfoTip label="About CPU">Percent of one core.</InfoTip>)
    const btn = screen.getByRole('button', { name: 'About CPU' })
    expect(btn).not.toHaveAttribute('aria-describedby')
    btn.focus()
    await userEvent.keyboard('{Enter}')
    const tip = await screen.findByText('Percent of one core.')
    expect(btn.getAttribute('aria-describedby')).toBe(tip.closest('[id]')?.id)
    await userEvent.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByText('Percent of one core.')).toBeNull(),
    )
  })
})

describe('Suggestion', () => {
  it('renders title, detail and calls an action', async () => {
    const onClick = vi.fn()
    render(
      <Suggestion
        title="No health check"
        detail="Found /healthz"
        actions={[{ label: 'Add it', onClick }]}
      />,
    )
    expect(screen.getByText('Found /healthz')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Add it' }))
    expect(onClick).toHaveBeenCalledTimes(1)
  })
  it('disables all actions while an async action is pending', async () => {
    let release: () => void = () => undefined
    const slow = vi.fn(
      () =>
        new Promise<void>((r) => {
          release = r
        }),
    )
    render(
      <Suggestion
        title="t"
        actions={[
          { label: 'Go', onClick: slow },
          { label: 'Later', kind: 'secondary', onClick: vi.fn() },
        ]}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Go' }))
    expect(screen.getByRole('button', { name: 'Go' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Go' })).toHaveAttribute(
      'aria-busy',
      'true',
    )
    expect(screen.getByRole('button', { name: 'Later' })).toBeDisabled()
    await act(() => {
      release()
      return Promise.resolve()
    })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Go' })).toBeEnabled(),
    )
    expect(slow).toHaveBeenCalledTimes(1)
  })
  it('honors the pending prop', () => {
    render(
      <Suggestion
        title="t"
        actions={[{ label: 'Go', pending: true, onClick: vi.fn() }]}
      />,
    )
    expect(screen.getByRole('button', { name: 'Go' })).toBeDisabled()
  })
  it('dismisses', async () => {
    const onDismiss = vi.fn()
    render(<Suggestion title="t" actions={[]} onDismiss={onDismiss} />)
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(onDismiss).toHaveBeenCalled()
  })
})

describe('SuggestionList', () => {
  const item = (id: string) => ({ id, title: `Title ${id}`, actions: [] })
  it('shows the empty label', () => {
    render(<SuggestionList items={[]} emptyLabel="All clear" />)
    expect(screen.getByText('All clear')).toBeInTheDocument()
  })
  it('keeps a removed item while it exits, then drops it', () => {
    vi.useFakeTimers()
    const { rerender } = render(
      <SuggestionList items={[item('a'), item('b')]} />,
    )
    rerender(<SuggestionList items={[item('a')]} />)
    expect(screen.getByText('Title b').closest('li')).toHaveAttribute(
      'data-exiting',
    )
    act(() => {
      vi.advanceTimersByTime(300)
    })
    expect(screen.queryByText('Title b')).toBeNull()
    expect(screen.getByText('Title a')).toBeInTheDocument()
  })
  it('removes immediately under reduced motion', () => {
    mockReducedMotion(true)
    const { rerender } = render(
      <SuggestionList items={[item('a'), item('b')]} />,
    )
    rerender(<SuggestionList items={[item('a')]} />)
    expect(screen.queryByText('Title b')).toBeNull()
  })
})

describe('EmptyState', () => {
  it('renders copy, action and illustration', () => {
    render(
      <EmptyState
        icon={<span>i</span>}
        illustration="rocket"
        title="Nothing yet"
        description="Deploy one."
        action={<button>Deploy</button>}
      />,
    )
    expect(screen.getByText('Nothing yet')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeInTheDocument()
    expect(
      document.querySelector('[data-illustration="rocket"]'),
    ).not.toBeNull()
  })
  it('falls back to the icon', () => {
    render(<EmptyState icon={<span>ICON</span>} title="t" />)
    expect(screen.getByText('ICON')).toBeInTheDocument()
  })
})

describe('ActionMenu', () => {
  const setup = () => {
    const onEdit = vi.fn()
    const onDelete = vi.fn()
    render(
      <ActionMenu
        trigger={<button>Actions</button>}
        items={[
          { id: 'del', label: 'Delete', tone: 'danger', onSelect: onDelete },
          {
            id: 'edit',
            label: 'Edit',
            onSelect: onEdit,
            description: 'Change it',
          },
          { id: 'off', label: 'Off', onSelect: vi.fn(), disabled: true },
        ]}
      />,
    )
    return { onEdit, onDelete }
  }
  it('opens with the keyboard and selects an item with Enter', async () => {
    const { onEdit } = setup()
    screen.getByRole('button', { name: 'Actions' }).focus()
    await userEvent.keyboard('{Enter}')
    const items = await screen.findAllByRole('menuitem')
    expect(items.map((i) => i.textContent)).toEqual([
      'EditChange it',
      'Off',
      'Delete',
    ])
    await userEvent.keyboard('{Enter}')
    expect(onEdit).toHaveBeenCalledTimes(1)
  })
  it('separates danger items and fires their handler', async () => {
    const { onDelete } = setup()
    await userEvent.click(screen.getByRole('button', { name: 'Actions' }))
    await screen.findAllByRole('menuitem')
    expect(
      document.querySelector('[data-slot="dropdown-menu-separator"]'),
    ).not.toBeNull()
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Delete' }),
    )
    expect(onDelete).toHaveBeenCalledTimes(1)
  })
  it('closes on Escape', async () => {
    setup()
    await userEvent.click(screen.getByRole('button', { name: 'Actions' }))
    await screen.findAllByRole('menuitem')
    await userEvent.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('menuitem')).toBeNull())
  })
})

describe('Timeline', () => {
  const base = { at: new Date(Date.now() - 240_000), icon: <span>ic</span> }
  it('renders items with relative time and actor', () => {
    render(
      <Timeline
        items={[
          { id: '1', ...base, title: 'Deployed', actor: 'gagan', detail: 'ok' },
        ]}
      />,
    )
    expect(screen.getByText('Deployed')).toBeInTheDocument()
    expect(screen.getByText('4m ago')).toBeInTheDocument()
    expect(screen.getByText(/gagan/)).toBeInTheDocument()
  })
  it('shows skeletons when loading and the empty label when empty', () => {
    const { rerender } = render(<Timeline items={[]} loading />)
    expect(screen.getAllByTestId('skeleton-row').length).toBeGreaterThan(0)
    rerender(<Timeline items={[]} emptyLabel="No activity" />)
    expect(screen.getByText('No activity')).toBeInTheDocument()
  })
  it('makes clickable rows keyboard accessible', async () => {
    const onClick = vi.fn()
    render(
      <Timeline
        items={[
          { id: '1', ...base, title: 'Plain' },
          { id: '2', ...base, title: 'Clickable', onClick },
        ]}
      />,
    )
    expect(screen.getAllByRole('button')).toHaveLength(1)
    await userEvent.tab()
    await userEvent.keyboard('{Enter}')
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})
