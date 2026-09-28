import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PipelineJob } from '../types/pipelines'
import { PipelineRunGraph } from './PipelineRunGraph'

function job(
  key: string,
  status: PipelineJob['status'],
  needs: string[] = [],
): PipelineJob {
  return { key, name: key, needs, status, attempt: 0, steps: [] }
}

describe('PipelineRunGraph', () => {
  it('draws one edge per dependency and reports selection', () => {
    const onSelect = vi.fn()
    const { container } = render(
      <PipelineRunGraph
        jobs={[
          job('test', 'succeeded'),
          job('ship', 'waiting_approval', ['test']),
        ]}
        selected="test"
        onSelect={onSelect}
      />,
    )
    expect(container.querySelectorAll('[data-edge]')).toHaveLength(1)
    expect(screen.getByRole('button', { name: /^test/ })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    const ship = screen.getByRole('button', { name: /^ship, Needs approval/ })
    fireEvent.click(ship)
    expect(onSelect).toHaveBeenCalledWith('ship')
  })

  it('selects with Enter and moves focus with arrow keys', () => {
    const onSelect = vi.fn()
    render(
      <PipelineRunGraph
        jobs={[job('a', 'succeeded'), job('b', 'pending', ['a'])]}
        selected="a"
        onSelect={onSelect}
      />,
    )
    const a = screen.getByRole('button', { name: /^a,/ })
    const b = screen.getByRole('button', { name: /^b,/ })
    a.focus()
    fireEvent.keyDown(a, { key: 'ArrowRight' })
    expect(document.activeElement).toBe(b)
    fireEvent.keyDown(b, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledWith('b')
  })

  it('stacks matrix jobs in one node with a button per member', () => {
    render(
      <PipelineRunGraph
        jobs={[
          job('test[go=1.22]', 'succeeded'),
          job('test[go=1.23]', 'failed'),
        ]}
        selected=""
        onSelect={() => undefined}
      />,
    )
    expect(screen.getAllByRole('button')).toHaveLength(2)
    expect(screen.getByText('2 runs')).toBeInTheDocument()
  })

  it('renders 120 jobs', () => {
    const jobs = Array.from({ length: 120 }, (_, i) =>
      job(`j${i}`, 'pending', i > 0 ? [`j${i - 1}`] : []),
    )
    render(
      <PipelineRunGraph jobs={jobs} selected="j0" onSelect={() => undefined} />,
    )
    expect(screen.getAllByRole('button')).toHaveLength(120)
  })
})
