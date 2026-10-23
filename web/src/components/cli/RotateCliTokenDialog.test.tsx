import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { stubFetch } from '../signin/signInTestUtils'
import { renderCli } from './cliTestUtils'
import { RotateCliTokenDialog } from './RotateCliTokenDialog'
import type { TokenResource } from '../../types/token'

const TOKEN: TokenResource = {
  id: 'tok-1',
  name: 'cli-pairing laptop',
  abilities: ['read'],
  created_at: '2026-01-01T00:00:00Z',
}
const REVOKE = 'DELETE /api/v1/auth/tokens/tok-1'
const PAIR = 'POST /api/v1/auth/cli-pairing'
const pairing = {
  status: 201,
  body: {
    code: 'WXYZ-1234',
    expires_at: new Date(Date.now() + 5 * 60_000).toISOString(),
    server_url: 'https://lr.example.test',
  },
}

afterEach(() => {
  vi.unstubAllGlobals()
})

function open(onClose = vi.fn()) {
  renderCli(<RotateCliTokenDialog token={TOKEN} onClose={onClose} />)
  return onClose
}

describe('RotateCliTokenDialog', () => {
  it('explains the consequence and does nothing until confirmed', async () => {
    const calls = stubFetch({})
    open()
    expect(await screen.findByText(/stops working as soon as/)).toBeVisible()
    expect(screen.getByText(/has to sign in again/)).toBeVisible()
    expect(calls).toEqual([])
  })

  it('revokes, then shows a fresh command and copies it', async () => {
    const calls = stubFetch({ [REVOKE]: { status: 200 }, [PAIR]: pairing })
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Revoke and get new command' }),
    )
    expect(await screen.findByTestId('rotate-cli-command')).toHaveTextContent(
      'testcli login --api-url https://lr.example.test --code WXYZ-1234',
    )
    expect(calls.indexOf(REVOKE)).toBeLessThan(calls.indexOf(PAIR))
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(
        'testcli login --api-url https://lr.example.test --code WXYZ-1234',
      )
    })
  })

  it('keeps the dialog on confirm and asks nothing new when revoke fails', async () => {
    const calls = stubFetch({ [REVOKE]: { status: 500, body: {} } })
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Revoke and get new command' }),
    )
    expect(await screen.findByText(/nothing changed/)).toBeVisible()
    expect(calls).toEqual([REVOKE])
  })

  it('says the token is revoked and offers a retry when pairing fails', async () => {
    stubFetch({
      [REVOKE]: { status: 200 },
      [PAIR]: { status: 500, body: {} },
    })
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Revoke and get new command' }),
    )
    expect(await screen.findByText(/old token is revoked/)).toBeVisible()
    expect(screen.queryByTestId('rotate-cli-command')).toBeNull()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeEnabled()
  })
})
