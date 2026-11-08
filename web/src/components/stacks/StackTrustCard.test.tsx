import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StackTrustCard } from './StackTrustCard'
import { jsonResponse, renderWithProviders } from './stacksTestUtils'

describe('StackTrustCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders nothing for a non-admin', () => {
    const { container } = renderWithProviders(
      <StackTrustCard name="shop" isRoot={false} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('lists trusts and posts a new one', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'POST'
          ? jsonResponse({ id: 't2' })
          : jsonResponse([
              {
                id: 't1',
                provider: 'github',
                repository: 'acme/infra',
                ref_pattern: 'refs/heads/main',
                environment: 'prod',
                verbs: ['plan'],
                created_by: 'ada',
                created_at: '2026-10-20T00:00:00Z',
              },
            ]),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderWithProviders(<StackTrustCard name="shop" isRoot />)
    expect(await screen.findByText('acme/infra')).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Repository'), 'acme/web')
    await userEvent.click(screen.getByRole('button', { name: 'Add trust' }))
    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'POST')).toBe(
        true,
      )
    })
  })
})
