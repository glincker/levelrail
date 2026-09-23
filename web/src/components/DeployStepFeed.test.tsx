import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DeployStepFeed } from './DeployStepFeed'
import type { DeployStepEvent } from '../hooks/useDeployStepStream'

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
})
