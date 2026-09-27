import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { DeployFailureView } from './DeployFailureView'
import type { DeployFailure } from '../types/deployFailure'

const failure: DeployFailure = {
  code: 'build_failed',
  cause: 'The image build failed.',
  failing_step: 'build',
  log_excerpt: 'npm ERR! missing script: build',
  suggested_fix: 'Add a build script to package.json.',
  docs_url: 'https://example.test/docs#build_failed',
  retryable: false,
  deploy_id: 'dep_1',
  app: 'web',
  at: '2026-09-26T00:00:00Z',
}

describe('DeployFailureView', () => {
  it('shows cause, step, fix and retry hint', () => {
    render(<DeployFailureView failure={failure} />)
    expect(screen.getByText('The image build failed.')).toBeInTheDocument()
    expect(screen.getByText(/failed at build/)).toBeInTheDocument()
    expect(
      screen.getByText('Add a build script to package.json.'),
    ).toBeInTheDocument()
    expect(screen.getByText(/will not help/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Read the docs' })).toHaveAttribute(
      'href',
      failure.docs_url,
    )
  })

  it('says retrying may succeed when retryable', () => {
    render(<DeployFailureView failure={{ ...failure, retryable: true }} />)
    expect(screen.getByText(/may succeed/)).toBeInTheDocument()
  })

  it('toggles the log excerpt from the keyboard with aria-expanded', async () => {
    const user = userEvent.setup()
    render(<DeployFailureView failure={failure} />)
    const toggle = screen.getByRole('button', { name: /log excerpt/i })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText(/missing script/)).not.toBeVisible()
    toggle.focus()
    await user.keyboard('{Enter}')
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText(/missing script/)).toBeVisible()
    await user.keyboard(' ')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
  })

  it('omits the excerpt toggle when there is no excerpt', () => {
    render(
      <DeployFailureView failure={{ ...failure, log_excerpt: undefined }} />,
    )
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders a busy status while loading', () => {
    render(<DeployFailureView loading />)
    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true')
  })

  it('renders nothing without a failure, or an empty note on request', () => {
    const { container, rerender } = render(<DeployFailureView />)
    expect(container).toBeEmptyDOMElement()
    rerender(<DeployFailureView showEmpty />)
    expect(screen.getByText(/No failure recorded/)).toBeInTheDocument()
  })
})
