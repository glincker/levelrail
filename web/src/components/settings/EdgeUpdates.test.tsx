import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import { UpgradePreflight } from './UpgradePreflight'
import { UpdateStatusRow } from './UpdateStatusRow'
import type {
  EdgeGuidance,
  UpdatePreflight,
  UpdateStatus,
} from '../../queries/updates'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const container: EdgeGuidance = {
  commit: '07cd3bf9a1b2',
  commit_url: 'https://github.com/x/y/commit/07cd3bf',
  image: 'ghcr.io/x/y:edge',
  runs_in_container: true,
  installable: true,
  pull_command: 'docker pull ghcr.io/x/y:edge',
  recreate_command: 'docker compose pull app && docker compose up -d app',
  compose_snippet: 'services:\n  app:\n    image: ghcr.io/x/y:edge\n',
}

const binary: EdgeGuidance = {
  commit: '07cd3bf9a1b2',
  commit_url: 'https://github.com/x/y/commit/07cd3bf',
  image: 'ghcr.io/x/y:edge',
  runs_in_container: false,
  installable: false,
  switch_to: ['beta', 'stable'],
}

function preflight(edge: EdgeGuidance): UpdatePreflight {
  return {
    current_version: 'v1.0.0',
    latest_version: 'main-07cd3bf',
    update_available: true,
    release_url: 'https://github.com/x/y/commit/07cd3bf',
    release_notes: '',
    checks: [
      {
        code: 'release_signature',
        name: 'Release checksums and signature',
        status: 'skipped',
        message:
          'Edge builds are container images, not signed binaries, so there are no checksums to verify.',
      },
    ],
    blocked: false,
    upgrade_command: '',
    rollback_command: '',
    cosign_command: '',
    release_kind: 'edge_image',
    channel: 'edge',
    edge,
  }
}

const fetchMock = vi.fn()

function renderPreflight(data: UpdatePreflight) {
  fetchMock.mockImplementation((_url: string, init?: RequestInit) =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve(
          init?.method === 'PUT'
            ? { channel: 'beta', auto_update_enabled: true }
            : data,
        ),
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <UpgradePreflight />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  fetchMock.mockReset()
})

describe('edge channel messaging', () => {
  it('explains the skipped signature check instead of blocking', async () => {
    renderPreflight(preflight(container))
    expect(
      await screen.findByText('Release checksums and signature'),
    ).toBeInTheDocument()
    expect(screen.getByText('Not applicable')).toBeInTheDocument()
    expect(screen.getAllByText(/not signed binaries/).length).toBeGreaterThan(0)
    expect(screen.queryByText(/Upgrade blocked/)).not.toBeInTheDocument()
  })

  it('shows the image, pull and recreate commands for a container install', async () => {
    renderPreflight(preflight(container))
    expect(
      await screen.findByText('docker pull ghcr.io/x/y:edge'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('docker compose pull app && docker compose up -d app'),
    ).toBeInTheDocument()
    expect(screen.getByText('View commit')).toHaveAttribute(
      'href',
      'https://github.com/x/y/commit/07cd3bf',
    )
    expect(screen.queryByText(/sh -s upgrade/)).not.toBeInTheDocument()
  })

  it('offers a one-click channel switch for a host binary install', async () => {
    renderPreflight(preflight(binary))
    expect(
      await screen.findByText('Edge cannot be installed on this host'),
    ).toBeInTheDocument()
    expect(screen.queryByText(/docker pull/)).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Switch to Beta (pre-release)' }),
    )
    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        (c) => (c[1] as RequestInit | undefined)?.method === 'PUT',
      )
      expect(put).toBeDefined()
      expect(
        JSON.parse((put?.[1] as RequestInit).body as string),
      ).toMatchObject({ channel: 'beta' })
    })
  })
})

describe('UpdateStatusRow', () => {
  const edgeStatus: UpdateStatus = {
    current_version: 'main-aaaaaaa',
    latest_version: 'main-07cd3bf',
    update_available: true,
    release_url: 'https://github.com/x/y/commit/07cd3bf',
    published_at: null,
    release_kind: 'edge_image',
    channel: 'edge',
    image: 'ghcr.io/x/y:edge',
  }

  it('links an edge build to its commit, not a release', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <UpdateStatusRow status={edgeStatus} />
      </I18nextProvider>,
    )
    expect(
      screen.getByText('A newer edge build is available: main-07cd3bf'),
    ).toBeInTheDocument()
    expect(screen.getByText('View commit')).toBeInTheDocument()
    expect(screen.queryByText('View release')).not.toBeInTheDocument()
    expect(screen.getByText('Image: ghcr.io/x/y:edge')).toBeInTheDocument()
  })

  it('keeps the release wording for a real release', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <UpdateStatusRow
          status={{
            ...edgeStatus,
            latest_version: 'v1.1.0',
            release_kind: 'release',
            channel: 'stable',
            image: null,
          }}
        />
      </I18nextProvider>,
    )
    expect(
      screen.getByText('A new version is available: v1.1.0'),
    ).toBeInTheDocument()
    expect(screen.getByText('View release')).toBeInTheDocument()
  })
})
