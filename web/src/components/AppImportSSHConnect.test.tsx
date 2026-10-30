import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppImportSSHConnect } from './AppImportSSHConnect'
import migrationEn from '../locales/en/migration.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

const FP = 'SHA256:' + 'A'.repeat(43)

interface Call {
  url: string
  body: Record<string, unknown>
}

function stubFetch(responses: Record<string, unknown>): Call[] {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({
        url,
        body:
          typeof init?.body === 'string'
            ? (JSON.parse(init.body) as Record<string, unknown>)
            : {},
      })
      const key = Object.keys(responses).find((k) => url.endsWith(k)) ?? ''
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(responses[key]),
      } as unknown as Response)
    }),
  )
  return calls
}

function renderConnect(onCreated = vi.fn()) {
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <AppImportSSHConnect onCreated={onCreated} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return onCreated
}

async function fillLogin(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Server address'), '203.0.113.9')
  await user.type(screen.getByLabelText('Private key'), 'KEY-MATERIAL')
}

afterEach(() => vi.unstubAllGlobals())

describe('AppImportSSHConnect', () => {
  it('blocks testing and connecting until the host key is confirmed', async () => {
    stubFetch({})
    const user = userEvent.setup()
    renderConnect()
    await fillLogin(user)
    const test = screen.getByRole('button', { name: 'Test connection' })
    const connect = screen.getByRole('button', { name: 'Read inventory' })
    expect(test).toBeDisabled()
    expect(connect).toBeDisabled()
    await user.type(screen.getByLabelText('Host key fingerprint'), FP)
    expect(test).toBeDisabled()
    await user.click(
      screen.getByRole('checkbox', {
        name: 'This fingerprint matches my old server',
      }),
    )
    expect(test).toBeEnabled()
    expect(connect).toBeEnabled()
  })

  it('reads the host key, pins the chosen one and sends the login once', async () => {
    const calls = stubFetch({
      '/ssh/host-keys': {
        ok: true,
        host_keys: [{ type: 'ssh-ed25519', fingerprint: FP }],
      },
      '/ssh/test': {
        ok: true,
        reads: 'read only: lists containers, volumes and images',
        overview: {
          docker_version: '27.3.1',
          swarm: false,
          containers: 5,
          running: 4,
          volumes: 3,
          images: 7,
        },
      },
      '/sessions': { id: 'appimp-1' },
    })
    const user = userEvent.setup()
    const onCreated = renderConnect()
    await fillLogin(user)
    await user.click(screen.getByRole('button', { name: 'Read host key' }))
    await user.click(await screen.findByRole('button', { name: /ssh-ed25519/ }))
    expect(screen.getByLabelText('Host key fingerprint')).toHaveValue(FP)
    await user.click(
      screen.getByRole('checkbox', {
        name: 'This fingerprint matches my old server',
      }),
    )
    await user.click(screen.getByRole('button', { name: 'Test connection' }))
    expect(
      await screen.findByText(/Docker 27.3.1: 5 containers \(4 running\)/),
    ).toBeInTheDocument()
    expect(
      screen.getAllByText('read only: lists containers, volumes and images')
        .length,
    ).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: 'Read inventory' }))
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith('appimp-1'))
    const create = calls.find((c) => c.url.endsWith('/sessions'))
    const ssh = create?.body.ssh as Record<string, unknown>
    expect(create?.body.platform).toBe('docker-ssh')
    expect(ssh.host_key_fingerprint).toBe(FP)
    expect(ssh.private_key).toBe('KEY-MATERIAL')
    expect(ssh.insecure_host_key).toBeUndefined()
    const hostKeyCall = calls.find((c) => c.url.endsWith('/ssh/host-keys'))
    expect(
      (hostKeyCall?.body.ssh as Record<string, unknown>).private_key,
    ).toBeUndefined()
    expect(screen.getByLabelText('Private key')).toHaveValue('')
  })

  it('shows a typed fix and re-checks after the socket path is corrected', async () => {
    const calls = stubFetch({
      '/ssh/test': {
        ok: false,
        kind: 'socket_path',
        message: 'there is no docker socket at /var/run/docker.sock',
      },
    })
    const user = userEvent.setup()
    renderConnect()
    await fillLogin(user)
    await user.type(screen.getByLabelText('Host key fingerprint'), FP)
    await user.click(
      screen.getByRole('checkbox', {
        name: 'This fingerprint matches my old server',
      }),
    )
    await user.click(screen.getByRole('button', { name: 'Test connection' }))
    expect(
      await screen.findByText('No Docker socket at that path'),
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Try /run/docker.sock' }),
    )
    await waitFor(() =>
      expect(calls.filter((c) => c.url.endsWith('/ssh/test'))).toHaveLength(2),
    )
    const second = calls.filter((c) => c.url.endsWith('/ssh/test'))[1]
    expect((second?.body.ssh as Record<string, unknown>).socket_path).toBe(
      '/run/docker.sock',
    )
  })

  it('offers the observed fingerprint on a host key mismatch', async () => {
    stubFetch({
      '/ssh/test': {
        ok: false,
        kind: 'host_key_mismatch',
        observed_host_keys: [
          { type: 'ssh-ed25519', fingerprint: 'SHA256:other' },
        ],
      },
    })
    const user = userEvent.setup()
    renderConnect()
    await fillLogin(user)
    await user.type(screen.getByLabelText('Host key fingerprint'), FP)
    await user.click(
      screen.getByRole('checkbox', {
        name: 'This fingerprint matches my old server',
      }),
    )
    await user.click(screen.getByRole('button', { name: 'Test connection' }))
    await user.click(
      await screen.findByRole('button', { name: 'Trust SHA256:other' }),
    )
    expect(screen.getByLabelText('Host key fingerprint')).toHaveValue(
      'SHA256:other',
    )
    expect(
      screen.getByRole('checkbox', {
        name: 'This fingerprint matches my old server',
      }),
    ).not.toBeChecked()
  })

  it('warns loudly when host key verification is skipped', async () => {
    stubFetch({})
    const user = userEvent.setup()
    renderConnect()
    await fillLogin(user)
    await user.click(
      screen.getByRole('checkbox', {
        name: 'Skip host key verification (unsafe)',
      }),
    )
    expect(screen.getByText('Host key verification is off')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Test connection' }),
    ).toBeEnabled()
  })
})
