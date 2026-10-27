import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { AllI18n } from '../test/allI18n'
import { DeployFailureDetails } from './DeployFailureDetails'
import type { DeployFailure } from '../types/deployFailure'

const failure: DeployFailure = {
  code: 'build_failed',
  cause: 'The image build failed.',
  failing_step: 'build',
  log_excerpt: 'npm ERR! missing script: build',
  suggested_fix: 'Add a build script.',
  docs_url: '',
  retryable: false,
  deploy_id: 'dep_1',
  app: 'web',
  at: '2026-09-26T00:00:00Z',
}

describe('DeployFailureDetails', () => {
  it('shows the class label and reveals a copyable excerpt', async () => {
    const user = userEvent.setup()
    render(
      <AllI18n>
        <DeployFailureDetails failure={failure} />
      </AllI18n>,
    )
    expect(screen.getByText('Build failed')).toBeInTheDocument()
    const toggle = screen.getByRole('button', { name: /error excerpt/i })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText(/missing script/)).not.toBeInTheDocument()
    await user.click(toggle)
    expect(screen.getByText(/missing script/)).toBeVisible()
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument()
  })

  it('renders nothing when failure data is absent', () => {
    const { container } = render(
      <AllI18n>
        <DeployFailureDetails />
      </AllI18n>,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
