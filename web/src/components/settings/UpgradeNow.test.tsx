import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type { SelfUpgradePlan } from '../../queries/selfUpgrade'
import type { UpdatePreflight } from '../../queries/updates'
import type { UpgradeScheduleStatus } from '../../queries/upgradeSchedule'
import { UpgradeNow } from './UpgradeNow'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const green: UpdatePreflight = {
  current_version: 'v1.0.0',
  latest_version: 'v1.1.0',
  update_available: true,
  release_url: null,
  release_notes: '',
  checks: [
    {
      code: 'control_plane_backup',
      name: 'Recent control plane backup',
      status: 'ok',
      message: 'newest backup is 5m old',
    },
  ],
  blocked: false,
  upgrade_command: 'sh -s upgrade',
  rollback_command: 'restore-snapshot --list',
}

const staleBackup: UpdatePreflight = {
  ...green,
  blocked: true,
  checks: [
    {
      code: 'control_plane_backup',
      name: 'Recent control plane backup',
      status: 'fail',
      message: 'newest backup is 30h old; upgrades need one newer than 1d 2h',
      kind: 'backup_stale',
      hint: 'Take a fresh backup now. It takes a few seconds.',
      protects: 'Gives the upgrade a restore point.',
      fix: { action: 'backup_now' },
    },
  ],
}

const olderThanDatabase: UpdatePreflight = {
  ...green,
  latest_version: 'v0.6.0',
  update_available: false,
  blocked: true,
  checks: [
    {
      code: 'schema_compat',
      name: 'Compatible with your database',
      status: 'fail',
      message:
        'target release is older than the database schema: database is at schema 2160, v0.6.0 supports 1010',
      kind: 'schema_older',
      hint: 'This release is older than your database. Stay on your current version, or use Roll back to go to an older release on purpose.',
      fix: { action: 'open_rollback' },
    },
  ],
}

const publishing: UpdatePreflight = {
  ...green,
  blocked: true,
  waiting: true,
  checks: [
    {
      code: 'release_signature',
      name: 'Release checksums and signature',
      status: 'pending',
      message: 'this release is still being published',
      kind: 'release_incomplete',
      retryable: true,
      fix: { action: 'recheck' },
    },
  ],
}

const plan: SelfUpgradePlan = {
  current_version: 'v1.0.0',
  target_version: 'v1.1.0',
  breaking: [
    {
      id: 'ports',
      version: 'v1.1.0',
      summary: 'Ingress ports move',
      requires_ack: true,
    },
  ],
  notes_available: true,
  can_apply: true,
  command: 'sudo levelrail self-upgrade --to v1.1.0',
  steps: ['download', 'health'],
}

const noSchedule: UpgradeScheduleStatus = {
  available: true,
  server_timezone: 'Europe/Berlin',
  now: '2026-10-11T12:00:00Z',
  active: null,
  recent: [],
  next_window: null,
}

interface Calls {
  method: string
  url: string
  body: string
}

function setup(opts: {
  preflight: UpdatePreflight
  afterBackup?: UpdatePreflight
  afterRecheck?: UpdatePreflight
  plan?: SelfUpgradePlan
  schedule?: UpgradeScheduleStatus
}) {
  const calls: Calls[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({
        method,
        url,
        body: typeof init?.body === 'string' ? init.body : '',
      })
      const ok = (body: unknown, status = 200) =>
        Promise.resolve({
          ok: true,
          status,
          json: () => Promise.resolve(body),
          clone: () => ({ json: () => Promise.resolve(body) }),
        })
      if (method === 'POST' && url.endsWith('/updates/backup-now')) {
        return ok(
          { backup: { name: 'b.db' }, preflight: opts.afterBackup ?? green },
          201,
        )
      }
      if (method === 'POST' && url.endsWith('/updates/self-upgrade')) {
        return ok({ status: 'started', target_version: 'v1.1.0' }, 202)
      }
      if (method === 'POST' && url.endsWith('/updates/schedule')) {
        return ok({ id: 'us_1', state: 'pending' }, 201)
      }
      if (method === 'DELETE' && url.endsWith('/updates/schedule')) {
        return ok({ id: 'us_1', state: 'cancelled' })
      }
      if (url.includes('/updates/preflight?refresh=1')) {
        return ok(opts.afterRecheck ?? green)
      }
      if (url.endsWith('/updates/preflight')) return ok(opts.preflight)
      if (url.includes('/self-upgrade/plan')) return ok(opts.plan ?? plan)
      if (url.includes('/self-upgrade/attempts')) return ok({ attempts: [] })
      if (url.endsWith('/updates/schedule')) {
        return ok(opts.schedule ?? noSchedule)
      }
      if (url.endsWith('/api/v1/updates')) {
        return ok({
          current_version: 'v1.0.0',
          latest_version: 'v1.1.0',
          update_available: true,
          release_url: null,
          published_at: null,
        })
      }
      return ok({})
    }),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <UpgradeNow />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return calls
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('UpgradeNow', () => {
  it('shows the development build hero instead of a blocked card', async () => {
    setup({
      preflight: {
        ...olderThanDatabase,
        state: 'ahead_of_releases',
        db_schema: 2690,
        rollback_version: 'v0.6.0',
      },
    })
    expect(
      await screen.findByText(
        /You are running a development build \(schema 2690\)/,
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Roll back to v0.6.0' }),
    ).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Check again' })).toBeEnabled()
    expect(screen.queryByText(/Not offering/)).toBeNull()
    expect(screen.queryByRole('button', { name: /Upgrade to/ })).toBeNull()
  })

  it('disables Roll back with a reason when no restore point exists', async () => {
    setup({ preflight: { ...olderThanDatabase, state: 'ahead_of_releases' } })
    expect(
      await screen.findByRole('button', { name: 'Roll back' }),
    ).toBeDisabled()
    expect(
      screen.getByText(/No restore point for an older version/),
    ).toBeInTheDocument()
  })

  it('previews the plan for a compatible upgrade', async () => {
    setup({
      preflight: {
        ...green,
        state: 'upgrade_available',
        migrations_to_apply: 3,
        restore_point_first: true,
        verify_window_seconds: 180,
      },
    })
    expect(await screen.findByText('v1.0.0 to v1.1.0')).toBeInTheDocument()
    expect(
      screen.getByText('3 database migrations to apply'),
    ).toBeInTheDocument()
    expect(screen.getByText(/restore point is taken first/)).toBeInTheDocument()
    expect(screen.getByText(/180 seconds/)).toBeInTheDocument()
  })

  it('explains that the next upgrade verifies and pins, with no command to run', async () => {
    setup({ preflight: green, plan: { ...plan, signature_pinned: false } })
    expect(
      await screen.findByText(/The next upgrade checks the signature/),
    ).toBeInTheDocument()
    expect(screen.queryByText(/cosign/i)).not.toBeInTheDocument()
  })

  it('shows the signed-releases pin once a signature was verified', async () => {
    setup({
      preflight: green,
      plan: {
        ...plan,
        signature_pinned: true,
        signature_pinned_version: 'v0.9.0',
      },
    })
    expect(
      await screen.findByText(/Signed releases only, pinned since v0.9.0/),
    ).toBeInTheDocument()
    expect(screen.queryByText(/not signature verified yet/)).toBeNull()
  })

  it('clears a stale backup blocker with Back up now and then allows the upgrade', async () => {
    const calls = setup({ preflight: staleBackup, afterBackup: green })

    const upgrade = await screen.findByRole('button', {
      name: 'Upgrade to v1.1.0 now',
    })
    expect(upgrade).toBeDisabled()
    expect(
      screen.getByText(/newest backup is 30h old; upgrades need one newer/),
    ).toBeInTheDocument()
    expect(screen.getByText(/Protects: Gives the upgrade/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Back up now' }))

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Upgrade to v1.1.0 now' }),
      ).toBeEnabled()
    })
    expect(
      screen.queryByRole('button', { name: 'Back up now' }),
    ).not.toBeInTheDocument()
    expect(
      calls.filter(
        (c) => c.method === 'POST' && c.url.endsWith('/updates/backup-now'),
      ),
    ).toHaveLength(1)
  })

  it('shows a half published release as waiting with a working Re-check', async () => {
    const calls = setup({ preflight: publishing, afterRecheck: green })

    expect(
      await screen.findByText('Waiting for the release to finish publishing'),
    ).toBeInTheDocument()
    expect(screen.getByText('Waiting')).toBeInTheDocument()
    expect(screen.queryByText('Blocked')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Re-check' }))
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Upgrade to v1.1.0 now' }),
      ).toBeEnabled()
    })
    expect(
      calls.some((c) => c.url.includes('/updates/preflight?refresh=1')),
    ).toBe(true)
  })

  it('needs each breaking change acknowledged before it upgrades', async () => {
    const calls = setup({ preflight: green })
    await userEvent.click(
      await screen.findByRole('button', { name: 'Upgrade to v1.1.0 now' }),
    )
    const dialog = await screen.findByRole('dialog')
    const confirm = within(dialog).getByRole('button', {
      name: 'Upgrade now',
    })
    expect(confirm).toBeDisabled()
    expect(
      within(dialog).getByText(/backup of the control plane/),
    ).toBeVisible()
    expect(
      within(dialog).getByText(/previous program and its data/),
    ).toBeVisible()

    await userEvent.click(
      within(dialog).getByRole('checkbox', { name: /Ingress ports move/ }),
    )
    await waitFor(() => {
      expect(confirm).toBeEnabled()
    })
    await userEvent.click(confirm)

    await waitFor(() => {
      expect(
        calls.filter(
          (c) => c.method === 'POST' && c.url.endsWith('/updates/self-upgrade'),
        ),
      ).toHaveLength(1)
    })
    const post = calls.find((c) => c.url.endsWith('/updates/self-upgrade'))
    expect(JSON.parse(post?.body ?? '{}')).toEqual({
      target: 'v1.1.0',
      ack: ['ports'],
    })
    expect(await screen.findByText('Upgrading to v1.1.0')).toBeInTheDocument()
  })

  it('schedules in the server time zone', async () => {
    const calls = setup({ preflight: green, plan: { ...plan, breaking: [] } })
    await userEvent.click(
      await screen.findByRole('button', { name: 'Schedule upgrade' }),
    )
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/Europe\/Berlin/)).toBeInTheDocument()
    const submit = within(dialog).getByRole('button', {
      name: 'Schedule upgrade',
    })
    expect(submit).toBeDisabled()

    const at = within(dialog).getByLabelText(/Date and time/)
    await userEvent.type(at, '2026-10-12T03:00')
    await waitFor(() => {
      expect(submit).toBeEnabled()
    })
    await userEvent.click(submit)

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === 'POST' && c.url.endsWith('/updates/schedule'),
        ),
      ).toBe(true)
    })
    const post = calls.find(
      (c) => c.method === 'POST' && c.url.endsWith('/updates/schedule'),
    )
    expect(JSON.parse(post?.body ?? '{}')).toMatchObject({
      target: 'v1.1.0',
      at_local: '2026-10-12T03:00',
      timezone: 'Europe/Berlin',
      replace: false,
    })
  })

  it('shows a held schedule with its reason and cancels it', async () => {
    const calls = setup({
      preflight: green,
      plan: { ...plan, breaking: [] },
      schedule: {
        ...noSchedule,
        active: {
          id: 'us_1',
          target_version: 'v1.1.0',
          source: 'manual',
          mode: 'at',
          state: 'blocked',
          run_at: '2026-10-12T01:00:00Z',
          retry_until: '2026-10-12T03:00:00Z',
          timezone: 'Europe/Berlin',
          acked: [],
          major_acked: false,
          created_by: 'root',
          created_at: '2026-10-11T10:00:00Z',
          check_count: 1,
          last_reason: 'Free disk space: 10 MiB free',
        },
      },
    })
    expect(await screen.findByText('Held')).toBeInTheDocument()
    expect(screen.getByText(/Free disk space: 10 MiB free/)).toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: 'Cancel schedule' }),
    )
    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === 'DELETE' && c.url.endsWith('/updates/schedule'),
        ),
      ).toBe(true)
    })
  })

  it('shows the host command when the dashboard cannot apply', async () => {
    setup({
      preflight: green,
      plan: {
        ...plan,
        breaking: [],
        can_apply: false,
        cannot_apply_reason: 'not root',
      },
    })
    expect(
      await screen.findByText('sudo levelrail self-upgrade --to v1.1.0'),
    ).toBeInTheDocument()
    expect(screen.getByText(/not root/)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Upgrade to v1.1.0 now' }),
    ).toBeDisabled()
  })
})

describe('UpgradeNow on the edge channel', () => {
  const edgePreflight: UpdatePreflight = {
    ...green,
    latest_version: 'main-07cd3bf',
    release_kind: 'edge_image',
    channel: 'edge',
    checks: [
      {
        code: 'release_signature',
        name: 'Release checksums and signature',
        status: 'skipped',
        message: 'Edge builds are container images, not signed binaries.',
      },
    ],
    edge: {
      commit: '07cd3bf9a1b2',
      image: 'ghcr.io/x/y:edge',
      runs_in_container: true,
      installable: true,
      pull_command: 'docker pull ghcr.io/x/y:edge',
      recreate_command: 'docker compose pull app && docker compose up -d app',
    },
  }

  it('hides Upgrade now and Schedule and shows the pull guidance', async () => {
    setup({ preflight: edgePreflight })
    expect(
      await screen.findByText('docker pull ghcr.io/x/y:edge'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Upgrade to .* now/ }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Schedule upgrade' }),
    ).not.toBeInTheDocument()
  })

  it('keeps a young stable release pending with Re-check, never skipped', async () => {
    setup({ preflight: publishing })
    expect(
      await screen.findByRole('button', { name: 'Re-check' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Not applicable')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Schedule upgrade' }),
    ).toBeInTheDocument()
  })
})
