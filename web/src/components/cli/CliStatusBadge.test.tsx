import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { stubFetch } from '../signin/signInTestUtils'
import { renderCli } from './cliTestUtils'
import { CliStatusBadge } from './CliStatusBadge'

const future = '2099-01-01T00:00:00Z'
const URL = 'GET /api/v1/auth/my-credentials'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('CliStatusBadge', () => {
  it('shows the badge with an attention dot when no CLI is connected', async () => {
    stubFetch({
      [URL]: {
        body: {
          sessions: [],
          tokens: [{ id: 't1', name: 'ci-deploy', created_at: future }],
        },
      },
    })
    renderCli(<CliStatusBadge />)
    expect(await screen.findByRole('button')).toBeInTheDocument()
    expect(screen.getByTestId('cli-badge-dot')).toBeInTheDocument()
  })

  it('renders nothing while the credentials are loading', () => {
    stubFetch({})
    const { container } = renderCli(<CliStatusBadge />)
    expect(container).toBeEmptyDOMElement()
  })

  it('is hidden once a CLI is connected', async () => {
    const calls = stubFetch({
      [URL]: {
        body: {
          sessions: [],
          tokens: [
            {
              id: 't1',
              name: 'cli-pairing laptop',
              created_at: future,
              expires_at: future,
            },
          ],
        },
      },
    })
    const { container } = renderCli(<CliStatusBadge />)
    await vi.waitFor(() => {
      expect(calls).toContain(URL)
    })
    await new Promise((r) => setTimeout(r, 20))
    expect(container).toBeEmptyDOMElement()
  })

  it('is hidden when the user may not read credentials', async () => {
    const calls = stubFetch({ [URL]: { status: 403, body: { error: 'no' } } })
    const { container } = renderCli(<CliStatusBadge />)
    await vi.waitFor(() => {
      expect(calls).toContain(URL)
    })
    await new Promise((r) => setTimeout(r, 20))
    expect(container).toBeEmptyDOMElement()
  })
})
