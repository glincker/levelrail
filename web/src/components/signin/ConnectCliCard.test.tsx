import { fireEvent, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConnectCliCard } from './ConnectCliCard'
import { renderWithProviders, stubFetch } from './signInTestUtils'

const future = () => new Date(Date.now() + 5 * 60_000).toISOString()

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ConnectCliCard', () => {
  it('shows nothing secret until asked, then one copy-paste command', async () => {
    const calls = stubFetch({
      'POST /api/v1/auth/cli-pairing': {
        status: 201,
        body: {
          code: 'ABCD-EF23',
          expires_at: future(),
          server_url: 'https://lr.example.test',
        },
      },
    })
    renderWithProviders(<ConnectCliCard />)
    expect(screen.queryByTestId('connect-cli-command')).toBeNull()
    expect(calls).toHaveLength(0)
    fireEvent.click(screen.getByRole('button', { name: 'Show my command' }))
    const cmd = await screen.findByTestId('connect-cli-command')
    expect(cmd).toHaveTextContent(
      'testcli login --api-url https://lr.example.test --code ABCD-EF23',
    )
    expect(screen.getByText(/Single use, expires in/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
  })

  it('offers a new command once the code has expired', async () => {
    stubFetch({
      'POST /api/v1/auth/cli-pairing': {
        status: 201,
        body: {
          code: 'ABCD-EF23',
          expires_at: new Date(Date.now() - 1000).toISOString(),
          server_url: 'https://lr.example.test',
        },
      },
    })
    renderWithProviders(<ConnectCliCard />)
    fireEvent.click(screen.getByRole('button', { name: 'Show my command' }))
    expect(await screen.findByText(/has expired/)).toBeInTheDocument()
    expect(screen.queryByTestId('connect-cli-command')).toBeNull()
    expect(screen.getByRole('button', { name: 'New command' })).toBeEnabled()
  })

  it('can be dismissed and stays dismissed', () => {
    stubFetch({})
    const first = renderWithProviders(<ConnectCliCard />)
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryByText('Connect your CLI')).toBeNull()
    first.unmount()
    renderWithProviders(<ConnectCliCard />)
    expect(screen.queryByText('Connect your CLI')).toBeNull()
  })
})
