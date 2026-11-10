import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../test/allI18n'
import { PlatformImportCard } from './PlatformImportCard'
import type { PlatformImportReport } from '../queries/platformImport'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))

const secretLooking = 'fixture-secret-aws-value'

const plan: PlatformImportReport = {
  platform: 'dokploy',
  counts: {},
  items: [],
  backups: {
    targets: [
      {
        source_name: 'Offsite R2',
        name: 'offsite-r2',
        provider: 'r2',
        endpoint: 'https://acct.r2.cloudflarestorage.com',
        bucket: 'shop-backups',
        state: 'needs_secret',
        detail:
          "keys not imported; run: backup-targets create --name 'offsite-r2'",
        has_keys: true,
      },
    ],
    schedules: [
      {
        database: 'shop-db',
        target: 'offsite-r2',
        cron: '0 3 * * *',
        retain: 7,
        paused: true,
        state: 'would_create',
      },
    ],
  },
  channels: {
    channels: [
      {
        source_name: 'Ops Slack',
        name: 'ops-slack',
        kind: 'slack',
        enabled: false,
        state: 'needs_secret',
        has_secret: true,
      },
    ],
    unsupported: [
      {
        source_name: 'Matrix',
        source_kind: 'matrix',
        reason: 'no matching channel kind here',
      },
    ],
  },
}

const applied: PlatformImportReport = {
  ...plan,
  journal: [
    {
      kind: 'backup_target',
      name: 'offsite-r2',
      id: 'bt-1',
      fingerprint: 'sha256:ab',
    },
  ],
}

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PlatformImportExtras', () => {
  it('renders the plan, maps toggles to the payload and undoes by journal', async () => {
    const calls: { url: string; body: Record<string, unknown> }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init: RequestInit) => {
        calls.push({
          url,
          body: JSON.parse(init.body as string) as Record<string, unknown>,
        })
        if (url.endsWith('/discover'))
          return Promise.resolve(jsonResponse(plan))
        if (url.endsWith('/apply'))
          return Promise.resolve(jsonResponse(applied))
        return Promise.resolve(
          jsonResponse({
            items: [
              { kind: 'backup_target', name: 'offsite-r2', action: 'removed' },
            ],
          }),
        )
      }),
    )
    render(
      <AllI18n>
        <QueryClientProvider client={createTestQueryClient()}>
          <PlatformImportCard />
        </QueryClientProvider>
      </AllI18n>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Dokploy' }))
    fireEvent.change(screen.getByLabelText('Source URL'), {
      target: { value: 'https://dokploy.example.com' },
    })
    fireEvent.change(screen.getByLabelText('API key'), {
      target: { value: 'tok-fixture' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Discover' }))

    await screen.findByText('Backup target offsite-r2')
    expect(
      screen.getByText('acct.r2.cloudflarestorage.com'),
    ).toBeInTheDocument()
    expect(screen.getByText('Backup schedule for shop-db')).toBeInTheDocument()
    expect(
      screen.getByText('no matching channel kind here'),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Include credentials')).not.toBeChecked()
    expect(
      screen.getByLabelText('Create alert channels enabled'),
    ).not.toBeChecked()

    fireEvent.click(screen.getByLabelText('Keep backup schedules running'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Import backups and channels only' }),
    )
    await screen.findByRole('button', { name: 'Undo 1 imported item' })
    const body = calls[1]?.body ?? {}
    expect(body.extras_only).toBe(true)
    expect(body.import_extras).toBe(true)
    expect(body.enable_backup_schedules).toBe(true)
    expect(body.include_credentials).toBeUndefined()
    expect(body.enable_imported_channels).toBeUndefined()

    fireEvent.click(
      screen.getByRole('button', { name: 'Undo 1 imported item' }),
    )
    await waitFor(() => expect(calls).toHaveLength(3))
    expect(calls[2]?.url).toBe('/api/v1/imports/platform/undo')
    expect(calls[2]?.body.journal).toEqual(applied.journal)
    expect(document.body.innerHTML).not.toContain(secretLooking)
    expect(document.body.innerHTML).not.toContain('tok-fixture')
  })
})
