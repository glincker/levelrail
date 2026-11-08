import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../locales/en/common.json'
import nodesEn from '../locales/en/nodes.json'
import { TooltipProvider } from '@/components/ui/tooltip'
import { createTestQueryClient } from '@/test/testQueryClient'
import type { PatchPreview, PatchRun } from '../queries/nodePatch'
import { NodePatchStatusCard } from './NodePatchStatusCard'

vi.hoisted(() => {
  import.meta.env.VITE_NODE_PATCH_POLL_MS = '40'
})

const abilities = vi.hoisted(() => ({ root: true }))
vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({
    role: null,
    can: () => abilities.root,
  }),
}))

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes', 'common'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const preview: PatchPreview = {
  node_id: 'local',
  manager: 'apt',
  include_all: false,
  packages: [
    {
      name: 'openssl',
      current_version: '3.0.1',
      new_version: '3.0.2',
      security: true,
      kernel: false,
      container_runtime: false,
    },
    {
      name: 'linux-image',
      current_version: '6.1',
      new_version: '6.2',
      security: false,
      kernel: true,
      container_runtime: false,
    },
  ],
  held: [
    {
      name: 'docker-ce',
      current_version: '24',
      new_version: '25',
      security: false,
      kernel: false,
      container_runtime: true,
    },
  ],
  other_available: 3,
  reboot_required_now: false,
  reboot_likely: true,
  preview_id: 'pv-1',
  expires_at: new Date(Date.now() + 600_000).toISOString(),
}

function makeRun(over: Partial<PatchRun> = {}): PatchRun {
  return {
    id: 'run-1',
    node_id: 'local',
    actor: 'admin@example.com',
    manager: 'apt',
    include_all: false,
    reboot_if_required: false,
    status: 'running',
    started_at: '2026-11-07T10:00:00Z',
    packages: ['openssl'],
    held: [],
    exit_code: 0,
    output_tail: 'Unpacking openssl',
    reboot_required: false,
    reboot_scheduled: false,
    ...over,
  }
}

interface Routes {
  runs?: () => PatchRun[]
  preview?: () => { status: number; body: unknown }
  apply?: () => { status: number; body: unknown }
}

function mockApi(routes: Routes) {
  const calls: { url: string; body: unknown }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const body: unknown =
        typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
      calls.push({ url, body })
      let out: { status: number; body: unknown } = {
        status: 200,
        body: { checked: true, total: 2, security: 1 },
      }
      if (url.endsWith('/patch/runs')) {
        out = { status: 200, body: routes.runs?.() ?? [] }
      } else if (url.endsWith('/patch/preview')) {
        out = routes.preview?.() ?? { status: 200, body: preview }
      } else if (url.endsWith('/patch/apply')) {
        out = routes.apply?.() ?? { status: 202, body: makeRun() }
      }
      return Promise.resolve({
        ok: out.status < 300,
        status: out.status,
        json: () => Promise.resolve(out.body),
      } as unknown as Response)
    }),
  )
  return calls
}

function renderCard(isLocal = true) {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <TooltipProvider>
        <NodePatchStatusCard nodeId="local" nodeName="main" isLocal={isLocal} />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

async function openPreview() {
  fireEvent.click(
    await screen.findByRole('button', { name: 'Preview updates' }),
  )
  await screen.findByText('openssl')
}

function confirmAndApply() {
  fireEvent.change(screen.getByLabelText(/Type main or APPLY/), {
    target: { value: 'APPLY' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Apply updates' }))
}

beforeEach(() => {
  abilities.root = true
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('NodePatchStatusCard', () => {
  it('shows the pending summary and empty history', async () => {
    mockApi({})
    renderCard()
    expect(await screen.findByText('2 updates available')).toBeTruthy()
    expect(screen.getByText('1 security')).toBeTruthy()
    expect(await screen.findByText(/No update runs yet/)).toBeTruthy()
  })

  it('is read-only with a plain note on a non-local node', async () => {
    const calls = mockApi({})
    renderCard(false)
    expect(
      await screen.findByText(/works only on the control plane host/),
    ).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Preview updates' })).toBeNull()
    expect(calls.some((c) => c.url.includes('/patch/runs'))).toBe(false)
  })

  it('disables the preview for callers without the root ability', async () => {
    abilities.root = false
    mockApi({})
    renderCard()
    const button = await screen.findByRole('button', {
      name: 'Preview updates',
    })
    expect(button.getAttribute('aria-disabled')).toBe('true')
    fireEvent.click(button)
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('lists packages and held packages, and gates Apply on the typed word', async () => {
    mockApi({})
    renderCard()
    await openPreview()
    expect(screen.getByText('linux-image')).toBeTruthy()
    expect(screen.getByText('Security')).toBeTruthy()
    expect(screen.getByText('Kernel')).toBeTruthy()
    expect(screen.getByText('docker-ce')).toBeTruthy()
    expect(screen.getByText('Container runtime')).toBeTruthy()
    expect(screen.getByText(/Preview expires in/)).toBeTruthy()
    const apply = screen.getByRole('button', { name: 'Apply updates' })
    expect((apply as HTMLButtonElement).disabled).toBe(true)
    fireEvent.change(screen.getByLabelText(/Type main or APPLY/), {
      target: { value: 'main' },
    })
    expect((apply as HTMLButtonElement).disabled).toBe(false)
  })

  it('re-previews with include_all when the held switch is turned on', async () => {
    const calls = mockApi({})
    renderCard()
    await openPreview()
    fireEvent.click(
      screen.getByRole('switch', { name: 'Include held packages' }),
    )
    await waitFor(() =>
      expect(
        calls.filter((c) => c.url.endsWith('/patch/preview')).at(-1)?.body,
      ).toEqual({ include_all: true, refresh: true }),
    )
  })

  it('applies and polls the run to success with the result', async () => {
    let polls = 0
    const calls = mockApi({
      runs: () => {
        polls += 1
        return [
          polls < 3
            ? makeRun()
            : makeRun({
                status: 'succeeded',
                output_tail: 'Done upgrading',
                patch_status_after: { total: 0, security: 0 },
              }),
        ]
      },
    })
    renderCard()
    await openPreview()
    confirmAndApply()
    expect(
      await screen.findByText('Succeeded', {}, { timeout: 5000 }),
    ).toBeTruthy()
    expect(
      screen.getByText(/Pending after the run: 0 total, 0 security/),
    ).toBeTruthy()
    expect(screen.getByText('Done upgrading')).toBeTruthy()
    const applyCall = calls.find((c) => c.url.endsWith('/patch/apply'))
    expect(applyCall?.body).toEqual({
      preview_id: 'pv-1',
      confirm: true,
      include_all: false,
      reboot_if_required: false,
    })
  })

  it.each([
    ['failed', 'Failed'],
    ['timed_out', 'Timed out'],
  ] as const)('shows a %s terminal run', async (status, label) => {
    mockApi({
      runs: () => [
        makeRun({ status, error: 'upgrade stopped', exit_code: 100 }),
      ],
    })
    renderCard()
    await openPreview()
    confirmAndApply()
    expect((await screen.findAllByText(label)).length).toBeGreaterThan(0)
    expect(await screen.findByText('upgrade stopped')).toBeTruthy()
  })

  it.each([
    ['drain_required', /Drain the node first/],
    ['patch_in_progress', /already in progress/],
    ['deploy_in_progress', /A deploy is running/],
    ['preview_expired', /preview expired/],
    ['preview_mismatch', /held packages choice changed/],
    ['preview_stale', /package list changed/],
    ['nothing_to_apply', /nothing to install/],
    ['runtime_upgrade_needs_systemd', /need systemd/],
  ])('maps the 409 %s to plain language', async (code, text) => {
    mockApi({
      apply: () => ({ status: 409, body: { error: `raw ${code}`, code } }),
    })
    renderCard()
    await openPreview()
    confirmAndApply()
    expect(await screen.findByText(text)).toBeTruthy()
    if (code === 'drain_required') {
      expect(screen.getByRole('button', { name: 'Open Drains' })).toBeTruthy()
    }
    if (code.startsWith('preview_')) {
      expect(
        screen.getByRole('button', { name: 'Refresh preview' }),
      ).toBeTruthy()
    }
  })

  it('explains a 501 from the preview', async () => {
    mockApi({
      preview: () => ({
        status: 501,
        body: { error: 'remote', code: 'patch_unsupported' },
      }),
    })
    renderCard()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview updates' }),
    )
    expect(
      await screen.findByText(
        'Applying updates works only on the control plane host for now.',
      ),
    ).toBeTruthy()
  })

  it('expands a history row to its details', async () => {
    mockApi({
      runs: () => [
        makeRun({
          status: 'succeeded',
          output_tail: 'all good',
          finished_at: '2026-11-07T10:05:00Z',
        }),
      ],
    })
    renderCard()
    const row = await screen.findByRole('button', { expanded: false })
    expect(screen.getByText('1 package')).toBeTruthy()
    fireEvent.click(row)
    expect(await screen.findByText('all good')).toBeTruthy()
  })
})
