import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DeployStepFeed } from './DeployStepFeed'
import type { DeployStepEvent } from '../hooks/useDeployStepStream'
import { DeployDetailI18n } from '@/test/deployDetailI18n'

describe('DeployStepFeed', () => {
  it('renders every known step even before any event arrives, all pending', () => {
    render(<DeployStepFeed steps={[]} />)
    expect(screen.getByText('Detecting framework')).toBeInTheDocument()
    expect(screen.getByText('Building')).toBeInTheDocument()
    expect(screen.getByText('Loading image')).toBeInTheDocument()
    expect(screen.getByText('Deploying')).toBeInTheDocument()
    expect(screen.queryByText('Failed')).not.toBeInTheDocument()
  })

  it('reflects each step event status without reordering the list', () => {
    const steps: DeployStepEvent[] = [
      { step: 'detecting', status: 'done', timestamp: 't1' },
      { step: 'building', status: 'running', timestamp: 't2' },
    ]
    const { container } = render(<DeployStepFeed steps={steps} />)
    const labels = Array.from(container.querySelectorAll('li span')).map(
      (el) => el.textContent,
    )
    expect(labels).toEqual([
      'Detecting framework',
      'Building',
      'Loading image',
      'Deploying',
    ])
  })

  it('shows a terminal Failed row once a failed step event arrives', () => {
    const steps: DeployStepEvent[] = [
      { step: 'detecting', status: 'done', timestamp: 't1' },
      { step: 'building', status: 'failed', timestamp: 't2' },
      { step: 'failed', status: 'failed', timestamp: 't3' },
    ]
    render(<DeployStepFeed steps={steps} />)
    expect(screen.getByText('Failed')).toBeInTheDocument()
    // Steps after the one that failed never got their own event, and
    // stay pending rather than being inferred as done or failed.
    expect(screen.getByText('Deploying')).toBeInTheDocument()
  })

  it('shows only the trace for a deploy with no build, with durations, reasons and pull progress', () => {
    const steps: DeployStepEvent[] = [
      {
        step: 'resolve_digest',
        status: 'done',
        timestamp: 't1',
        duration_ms: 420,
        detail:
          'registry docker.io, digest sha256:abc, unchanged since the last deploy',
      },
      {
        step: 'image_pull',
        status: 'running',
        timestamp: 't2',
        detail: '42%, 3/7 layers',
      },
      {
        step: 'image_pull_layer',
        key: 'image_pull_layer.abc',
        status: 'done',
        timestamp: 't3',
        detail: 'abc complete',
      },
      {
        step: 'cutover',
        status: 'skipped',
        timestamp: 't4',
        reason: 'NotCutOver',
        detail: 'previous container web-1 keeps serving',
      },
    ]
    render(
      <DeployDetailI18n>
        <DeployStepFeed steps={steps} />
      </DeployDetailI18n>,
    )
    expect(screen.queryByText('Detecting framework')).not.toBeInTheDocument()
    expect(screen.getByText('Resolve tag to digest')).toBeInTheDocument()
    expect(screen.getByText('420 ms')).toBeInTheDocument()
    expect(screen.getByText('NotCutOver')).toBeInTheDocument()
    expect(
      screen.getByText('previous container web-1 keeps serving'),
    ).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).toHaveAttribute('value', '42')
  })
})
