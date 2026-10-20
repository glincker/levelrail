import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { PipelineJob } from '../types/pipelines'
import { PipelineStepList } from './PipelineStepList'

const job: PipelineJob = {
  key: 'test',
  name: 'test',
  needs: [],
  status: 'failed',
  attempt: 0,
  steps: [
    {
      index: 0,
      name: 'Set up job',
      kind: 'setup',
      status: 'succeeded',
      attempt: 0,
      started_at: '2026-01-01T00:00:00Z',
      finished_at: '2026-01-01T00:00:05Z',
    },
    { index: 1, name: 'go test', kind: 'run', status: 'failed', attempt: 0 },
  ],
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('PipelineStepList', () => {
  it('shows each step duration and toggles the step filter', () => {
    const onPickStep = vi.fn()
    render(<PipelineStepList job={job} step={1} onPickStep={onPickStep} />)
    expect(screen.getByText('5s')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^Set up job/ }))
    expect(onPickStep).toHaveBeenLastCalledWith(0)
    fireEvent.click(screen.getByRole('button', { name: /^go test/ }))
    expect(onPickStep).toHaveBeenLastCalledWith(undefined)
  })

  it('copies a link that opens this job and step', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    render(<PipelineStepList job={job} onPickStep={() => undefined} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Copy link to step go test' }),
    )
    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1))
    const url = new URL(String(writeText.mock.calls[0]?.[0]))
    expect(url.searchParams.get('job')).toBe('test')
    expect(url.searchParams.get('step')).toBe('1')
  })
})
