import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AnimatedNumber } from './AnimatedNumber'
import { Kbd } from './Kbd'
import { RelativeTime } from './RelativeTime'
import { formatRelative } from './formatRelative'
import { SkeletonList } from './Skeleton'
import { areaPath, smoothPath, sparkPoints } from './sparkPath'
import { Sparkline } from './Sparkline'
import { StatusPill } from './StatusPill'
import { TypedText } from './TypedText'
import { mockReducedMotion } from './testUtils'

afterEach(() => {
  vi.useRealTimers()
  mockReducedMotion(false)
})

describe('StatusPill', () => {
  it('renders label, tone and title', () => {
    render(<StatusPill tone="success" label="Running" title="all good" />)
    const el = screen.getByText('Running')
    expect(el).toHaveAttribute('data-tone', 'success')
    expect(el).toHaveAttribute('title', 'all good')
  })
  it('pulses only when live', () => {
    const { rerender } = render(<StatusPill tone="info" label="x" />)
    expect(screen.queryByTestId('pill-pulse')).toBeNull()
    rerender(<StatusPill tone="info" label="x" live />)
    expect(screen.getByTestId('pill-pulse')).toBeInTheDocument()
  })
})

describe('Sparkline path generation', () => {
  it('handles empty series', () => {
    expect(sparkPoints([], 100, 20)).toEqual([])
    expect(smoothPath([])).toBe('')
    render(<Sparkline values={[]} ariaLabel="empty" />)
    expect(screen.getByRole('img', { name: 'empty' })).toBeInTheDocument()
    expect(screen.getByTestId('spark-empty')).toBeInTheDocument()
    expect(screen.queryByTestId('spark-line')).toBeNull()
  })
  it('handles a single point with a dot', () => {
    const pts = sparkPoints([5], 100, 20)
    expect(pts).toHaveLength(1)
    render(<Sparkline values={[5]} ariaLabel="one" />)
    expect(screen.getByTestId('spark-last')).toBeInTheDocument()
    expect(screen.queryByTestId('spark-line')).toBeNull()
  })
  it('centers a flat series and ignores non-finite values', () => {
    const pts = sparkPoints([3, NaN, 3, 3], 100, 20)
    expect(pts).toHaveLength(3)
    expect(new Set(pts.map((p) => p.y))).toEqual(new Set([10]))
  })
  it('keeps the smooth path inside the box', () => {
    const pts = sparkPoints([0, 100, 0, 100, 0], 100, 20)
    const d = smoothPath(pts, 20)
    expect(d.startsWith('M')).toBe(true)
    for (const m of d.matchAll(/C[\d.-]+ ([\d.-]+)/g)) {
      const y = Number(m[1])
      expect(y).toBeGreaterThanOrEqual(0)
      expect(y).toBeLessThanOrEqual(20)
    }
  })
  it('closes the area path and renders optional layers', () => {
    const pts = sparkPoints([1, 2, 3], 50, 20)
    const d = smoothPath(pts, 20)
    expect(areaPath(d, pts, 20).endsWith('Z')).toBe(true)
    expect(areaPath('', pts.slice(0, 1), 20)).toBe('')
    render(
      <Sparkline values={[1, 2, 3]} ariaLabel="s" fill showLast width="fill" />,
    )
    expect(screen.getByTestId('spark-area')).toBeInTheDocument()
    expect(screen.getByTestId('spark-last')).toBeInTheDocument()
  })
})

describe('AnimatedNumber', () => {
  it('tweens to the target', () => {
    vi.useFakeTimers()
    const { rerender } = render(<AnimatedNumber value={0} durationMs={200} />)
    rerender(<AnimatedNumber value={100} durationMs={200} />)
    act(() => {
      vi.advanceTimersByTime(100)
    })
    const mid = Number(screen.getByText(/\d/).textContent)
    expect(mid).toBeGreaterThan(0)
    expect(mid).toBeLessThan(100)
    act(() => {
      vi.advanceTimersByTime(400)
    })
    expect(screen.getByText('100')).toBeInTheDocument()
  })
  it('keeps decimals for decimal targets', () => {
    render(<AnimatedNumber value={2.5} />)
    expect(screen.getByText('2.5')).toBeInTheDocument()
  })
  it('jumps immediately under reduced motion', () => {
    mockReducedMotion(true)
    const { rerender } = render(<AnimatedNumber value={1} />)
    rerender(<AnimatedNumber value={50} />)
    expect(screen.getByText('50')).toBeInTheDocument()
  })
  it('uses a custom format', () => {
    render(<AnimatedNumber value={7} format={(n) => `${Math.round(n)}%`} />)
    expect(screen.getByText('7%')).toBeInTheDocument()
  })
})

describe('TypedText', () => {
  it('reveals progressively then calls onDone', () => {
    vi.useFakeTimers()
    const done = vi.fn()
    render(<TypedText text="hello" speedMs={10} onDone={done} />)
    expect(screen.getByLabelText('hello').textContent).toBe('')
    act(() => {
      vi.advanceTimersByTime(20)
    })
    expect(screen.getByLabelText('hello').textContent).toBe('he')
    expect(done).not.toHaveBeenCalled()
    act(() => {
      vi.advanceTimersByTime(100)
    })
    expect(screen.getByLabelText('hello').textContent).toBe('hello')
    expect(done).toHaveBeenCalledTimes(1)
  })
  it('completes instantly under reduced motion', () => {
    mockReducedMotion(true)
    const done = vi.fn()
    render(<TypedText text="hello" onDone={done} />)
    expect(screen.getByLabelText('hello').textContent).toBe('hello')
    expect(done).toHaveBeenCalled()
  })
})

describe('RelativeTime', () => {
  const now = Date.parse('2026-01-10T12:00:00Z')
  const cases: [number, string][] = [
    [5, 'just now'],
    [44, 'just now'],
    [60, '1m ago'],
    [240, '4m ago'],
    [3599, '60m ago'],
    [7200, '2h ago'],
    [86400 * 3, '3d ago'],
    [86400 * 60, '2mo ago'],
    [86400 * 800, '2y ago'],
    [-600, 'in 10m'],
  ]
  it.each(cases)('formats %d seconds as %s', (secs, expected) => {
    expect(formatRelative(new Date(now - secs * 1000), now)).toBe(expected)
  })
  it('returns empty for invalid dates', () => {
    expect(formatRelative('nope', now)).toBe('')
  })
  it('shows the full timestamp in title', () => {
    render(<RelativeTime at={new Date(Date.now() - 240_000)} />)
    const t = screen.getByText('4m ago')
    expect(t).toHaveAttribute('title')
    expect(t.getAttribute('title')).not.toBe('')
  })
  it('refreshes on an interval only when live', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-10T12:00:00Z'))
    const at = new Date('2026-01-10T11:56:00Z')
    render(<RelativeTime at={at} live />)
    expect(screen.getByText('4m ago')).toBeInTheDocument()
    act(() => {
      vi.advanceTimersByTime(3 * 60_000)
    })
    expect(screen.getByText('7m ago')).toBeInTheDocument()
  })
})

describe('Kbd and skeletons', () => {
  it('renders each key', () => {
    render(<Kbd keys={['Ctrl', 'K']} />)
    expect(screen.getByText('Ctrl').tagName).toBe('KBD')
    expect(screen.getByText('K').tagName).toBe('KBD')
  })
  it('renders the requested skeleton rows', () => {
    render(<SkeletonList rows={4} />)
    expect(screen.getAllByTestId('skeleton-row')).toHaveLength(4)
  })
})
