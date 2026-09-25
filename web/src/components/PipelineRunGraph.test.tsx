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
  it('renders one button per job with its dependencies and reports selection', () => {
    const onSelect = vi.fn()
    render(
      <PipelineRunGraph
        jobs={[
          job('test', 'succeeded'),
          job('ship', 'waiting_approval', ['test']),
        ]}
        selected="test"
        onSelect={onSelect}
      />,
    )
    expect(screen.getByRole('button', { name: /^test/ })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    const ship = screen.getByRole('button', { name: /^ship/ })
    expect(ship).toHaveTextContent('needs test')
    fireEvent.click(ship)
    expect(onSelect).toHaveBeenCalledWith('ship')
  })
})
