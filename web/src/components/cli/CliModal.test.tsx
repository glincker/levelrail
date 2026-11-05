import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { stubFetch } from '../signin/signInTestUtils'
import { renderCli } from './cliTestUtils'
import { CliModal } from './CliModal'
import { setCliModalOpen } from '../../lib/cliModalStore'
import { CLI_INSTALL_COMMAND } from '../../lib/cliInstall'
import { CLI_INSTALL_COMMAND_WINDOWS } from './cliPlatform'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const revokeMutate = vi.fn()
vi.mock('../../queries/tokens', () => ({
  useRevokeToken: () => ({ mutate: revokeMutate, isPending: false }),
}))

// Commands are tinted across spans, so match the whole line after the prompt.
const cmdLine = (text: string) => (_: string, el: Element | null) =>
  el?.tagName === 'SPAN' &&
  el.classList.contains('flex') &&
  el.textContent?.slice(1) === text

const future = () => new Date(Date.now() + 5 * 60_000).toISOString()
const CREDS = 'GET /api/v1/auth/my-credentials'

beforeEach(() => {
  revokeMutate.mockReset()
})

afterEach(() => {
  act(() => {
    setCliModalOpen(false)
  })
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function open() {
  act(() => {
    setCliModalOpen(true)
  })
}

describe('CliModal', () => {
  it('stays closed until opened, then shows install, connect and manage', async () => {
    stubFetch({ [CREDS]: { body: { sessions: [], tokens: [] } } })
    renderCli(<CliModal />)
    expect(screen.queryByRole('dialog')).toBeNull()
    open()
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveAccessibleName('Command line tool')
    expect(
      await screen.findByText(cmdLine(CLI_INSTALL_COMMAND)),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Show my command' }),
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByTestId('cli-status-chip')).toHaveTextContent(
        'Not connected',
      ),
    )
    expect(
      screen.getByRole('link', { name: 'Manage all CLI access' }),
    ).toHaveAttribute('href', '/settings/cli-access')
  })

  it('creates a pairing code and shows the login command', async () => {
    stubFetch({
      [CREDS]: { body: { sessions: [], tokens: [] } },
      'POST /api/v1/auth/cli-pairing': {
        status: 201,
        body: {
          code: 'ABCD-EF23',
          expires_at: future(),
          server_url: 'https://lr.example.test',
        },
      },
    })
    renderCli(<CliModal />)
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Show my command' }),
    )
    expect(
      await screen.findByText(
        cmdLine(
          'testcli login --api-url https://lr.example.test --code ABCD-EF23',
        ),
      ),
    ).toBeInTheDocument()
    expect(screen.getByText(/Single use, expires in/)).toBeInTheDocument()
    expect(
      screen.getByText('# Sign this terminal in to https://lr.example.test'),
    ).toBeInTheDocument()
  })

  it('shows an expired connect command and offers a new one', async () => {
    stubFetch({
      [CREDS]: { body: { sessions: [], tokens: [] } },
      'POST /api/v1/auth/cli-pairing': {
        status: 201,
        body: {
          code: 'ABCD-EF23',
          expires_at: new Date(Date.now() - 1000).toISOString(),
          server_url: 'https://lr.example.test',
        },
      },
    })
    renderCli(<CliModal />)
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Show my command' }),
    )
    expect(
      await screen.findByText('# That command expired, get a new one'),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('connect-cli-command')).toBeNull()
    expect(
      screen.getByRole('button', { name: 'New command' }),
    ).toBeInTheDocument()
  })

  it('picks the install command for the visitor OS', async () => {
    stubFetch({ [CREDS]: { body: { sessions: [], tokens: [] } } })
    vi.spyOn(window.navigator, 'platform', 'get').mockReturnValue('Win32')
    renderCli(<CliModal />)
    open()
    expect(
      await screen.findByText(cmdLine(CLI_INSTALL_COMMAND_WINDOWS)),
    ).toBeInTheDocument()
    expect(screen.queryByText(cmdLine(CLI_INSTALL_COMMAND))).toBeNull()
  })

  it('keeps the shell installer on macOS and Linux', async () => {
    stubFetch({ [CREDS]: { body: { sessions: [], tokens: [] } } })
    vi.spyOn(window.navigator, 'platform', 'get').mockReturnValue('MacIntel')
    renderCli(<CliModal />)
    open()
    expect(
      await screen.findByText(cmdLine(CLI_INSTALL_COMMAND)),
    ).toBeInTheDocument()
    expect(screen.queryByText(cmdLine(CLI_INSTALL_COMMAND_WINDOWS))).toBeNull()
  })

  it('fills the app name from the first app, else keeps the placeholder', async () => {
    stubFetch({
      [CREDS]: { body: { sessions: [], tokens: [] } },
      'GET /api/v1/apps': { body: [{ name: 'shop' }, { name: 'blog' }] },
    })
    renderCli(<CliModal />)
    open()
    expect(
      await screen.findByText(cmdLine('testcli apps logs shop --follow')),
    ).toBeInTheDocument()
    expect(screen.getByText(cmdLine('testcli open shop'))).toBeInTheDocument()
  })

  it('collapses install and connect once a CLI is connected', async () => {
    stubFetch({
      [CREDS]: {
        body: {
          sessions: [],
          tokens: [
            {
              id: 'tok-1',
              name: 'device: laptop',
              created_at: '2026-01-01T00:00:00Z',
              last_used_at: '2026-01-02T00:00:00Z',
            },
          ],
        },
      },
    })
    renderCli(<CliModal />)
    open()
    expect(await screen.findByText('device: laptop')).toBeInTheDocument()
    expect(screen.queryByText(cmdLine(CLI_INSTALL_COMMAND))).toBeNull()
    expect(screen.getByText(cmdLine('testcli status'))).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Connect another CLI' }))
    expect(
      await screen.findByText(cmdLine(CLI_INSTALL_COMMAND)),
    ).toBeInTheDocument()
  })

  it('shows a copied state after copying a block', async () => {
    stubFetch({ [CREDS]: { body: { sessions: [], tokens: [] } } })
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', {
      ...window.navigator,
      clipboard: { writeText },
    })
    renderCli(<CliModal />)
    open()
    await screen.findByText(cmdLine(CLI_INSTALL_COMMAND))
    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0]!)
    expect(await screen.findAllByText('Copied')).not.toHaveLength(0)
    expect(writeText).toHaveBeenCalledWith(CLI_INSTALL_COMMAND)
  })

  it('lists connected CLIs and revokes one after confirming', async () => {
    stubFetch({
      [CREDS]: {
        body: {
          sessions: [],
          tokens: [
            {
              id: 'tok-1',
              name: 'cli-pairing laptop',
              created_at: '2026-01-01T00:00:00Z',
              last_used_at: '2026-01-02T00:00:00Z',
              last_ip: '203.0.113.9',
              expires_at: '2099-01-01T00:00:00Z',
            },
            {
              id: 'tok-2',
              name: 'ci-deploy',
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
        },
      },
    })
    renderCli(<CliModal />)
    open()
    expect(await screen.findByText('cli-pairing laptop')).toBeInTheDocument()
    expect(screen.queryByText('ci-deploy')).toBeNull()
    expect(screen.getByTestId('cli-status-chip')).toHaveTextContent(
      '1 connected',
    )
    expect(screen.getByText(/203\.0\.113\.9/)).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Revoke cli-pairing laptop' }),
    )
    expect(revokeMutate).not.toHaveBeenCalled()
    const confirm = await screen
      .findByRole('alertdialog')
      .catch(() =>
        screen.getByText('Revoke this CLI?').closest('[role="dialog"]'),
      )
    fireEvent.click(
      within(confirm as HTMLElement).getByRole('button', { name: 'Revoke' }),
    )
    expect(revokeMutate).toHaveBeenCalledWith('tok-1', expect.any(Object))
  })

  it('offers Rotate on a CLI row and hides fixture tokens', async () => {
    stubFetch({
      [CREDS]: {
        body: {
          sessions: [],
          tokens: [
            {
              id: 'tok-1',
              name: 'cli-pairing laptop',
              created_at: '2026-01-01T00:00:00Z',
            },
            {
              id: 'fx',
              name: 'cli-pairing fixture',
              dev_fixture: true,
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
        },
      },
    })
    renderCli(<CliModal />)
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Rotate cli-pairing laptop' }),
    )
    expect(await screen.findByText('Rotate this CLI token?')).toBeVisible()
    expect(revokeMutate).not.toHaveBeenCalled()
    expect(screen.queryByText('cli-pairing fixture')).toBeNull()
  })

  it('lists device-flow and password tokens but hides other API tokens', async () => {
    const base = { created_at: '2026-01-01T00:00:00Z', abilities: ['read'] }
    stubFetch({
      [CREDS]: {
        body: {
          sessions: [],
          tokens: [
            {
              ...base,
              id: 'a',
              name: 'device: Mac.lan',
              abilities: ['read', 'write', 'deploy'],
            },
            {
              ...base,
              id: 'b',
              name: 'levelrail-cli-20260101-000000',
              abilities: ['read:sensitive'],
            },
            { ...base, id: 'c', name: 'cli-pairing', abilities: ['root'] },
            { ...base, id: 'd', name: 'github-actions' },
            { ...base, id: 'e', name: 'device-ish' },
          ],
        },
      },
    })
    renderCli(<CliModal />)
    open()
    expect(await screen.findByText('device: Mac.lan')).toBeInTheDocument()
    expect(
      screen.getByText('levelrail-cli-20260101-000000'),
    ).toBeInTheDocument()
    expect(screen.getByText('cli-pairing')).toBeInTheDocument()
    expect(screen.queryByText('github-actions')).toBeNull()
    expect(screen.queryByText('device-ish')).toBeNull()
    expect(screen.getByTestId('cli-status-chip')).toHaveTextContent(
      '3 connected',
    )
    expect(screen.getByText('Full access')).toBeInTheDocument()
    expect(screen.getByText('Sensitive')).toBeInTheDocument()
    expect(screen.getByText('Deploy')).toBeInTheDocument()
  })

  it('shows an expired CLI as expired and not connected', async () => {
    stubFetch({
      [CREDS]: {
        body: {
          sessions: [],
          tokens: [
            {
              id: 'x',
              name: 'device: old',
              abilities: ['read'],
              created_at: '2026-01-01T00:00:00Z',
              expires_at: '2026-01-02T00:00:00Z',
            },
          ],
        },
      },
    })
    renderCli(<CliModal />)
    open()
    expect(await screen.findByText('device: old')).toBeInTheDocument()
    expect(screen.getByText('Expired')).toBeInTheDocument()
    expect(screen.getByTestId('cli-status-chip')).toHaveTextContent(
      'Not connected',
    )
  })

  it('offers verified common commands with the brand binary name', async () => {
    stubFetch({ [CREDS]: { body: { sessions: [], tokens: [] } } })
    renderCli(<CliModal />)
    open()
    expect(
      await screen.findByText(cmdLine('testcli apps logs <app> --follow')),
    ).toBeInTheDocument()
    expect(screen.getByText(cmdLine('testcli status'))).toBeInTheDocument()
  })
})
