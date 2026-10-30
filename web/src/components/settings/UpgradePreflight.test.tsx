import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import { UpgradePreflight } from './UpgradePreflight'
import type { UpdatePreflight } from '../../queries/updates'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const base: UpdatePreflight = {
  current_version: 'v1.0.0',
  latest_version: 'v1.1.0',
  update_available: true,
  release_url: null,
  release_notes: 'Fixes things',
  checks: [
    {
      code: 'disk_space',
      name: 'Free disk space',
      status: 'ok',
      message: '9 GiB free',
    },
  ],
  blocked: false,
  upgrade_command: 'sh -s upgrade',
  rollback_command: 'restore-snapshot --list',
}

function renderWith(data: UpdatePreflight) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(data),
      }),
    ),
  )
  const qc = createTestQueryClient()
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
})

describe('UpgradePreflight', () => {
  it('shows checks and the copyable command when an update is available', async () => {
    renderWith(base)
    expect(await screen.findByText('Free disk space')).toBeInTheDocument()
    expect(screen.getByText('Pass')).toBeInTheDocument()
    expect(screen.getByText('sh -s upgrade')).toBeInTheDocument()
    expect(screen.getByText('Fixes things')).toBeInTheDocument()
  })

  it('hides the command and explains when blocked', async () => {
    renderWith({
      ...base,
      blocked: true,
      checks: [
        {
          code: 'docker_engine',
          name: 'Docker Engine version',
          status: 'fail',
          message: 'known bad',
        },
      ],
    })
    expect(await screen.findByText('Blocked')).toBeInTheDocument()
    expect(screen.getByText(/Upgrade blocked/)).toBeInTheDocument()
    expect(screen.queryByText('sh -s upgrade')).not.toBeInTheDocument()
  })

  it('shows plain retry copy and no terminal command while the trust root downloads', async () => {
    renderWith({
      ...base,
      checks: [
        {
          code: 'release_verifier',
          name: 'Release signature verification',
          status: 'warn',
          message:
            'Checking the release signature needs one download from Sigstore. Retrying automatically.',
          hint: 'This clears on its own once the server can reach Sigstore. You can also re-check.',
          fix: { action: 'recheck' },
        },
      ],
    })
    expect(
      await screen.findByText(/one download from Sigstore/),
    ).toBeInTheDocument()
    expect(screen.queryByText(/cosign/i)).not.toBeInTheDocument()
    expect(
      screen.getAllByRole('button', { name: 'Re-check' }).length,
    ).toBeGreaterThan(0)
  })

  it('shows a ready verifier without any fix', async () => {
    renderWith({
      ...base,
      checks: [
        {
          code: 'release_verifier',
          name: 'Release signature verification',
          status: 'ok',
          message: 'Release signatures are verified by this server.',
        },
      ],
    })
    expect(
      await screen.findByText('Release signature verification'),
    ).toBeInTheDocument()
    expect(screen.queryByText(/cosign/i)).not.toBeInTheDocument()
  })
})
