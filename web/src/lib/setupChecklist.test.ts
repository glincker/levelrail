import { describe, expect, it } from 'vitest'
import { computeSetupChecklist } from './setupChecklist'
import type { SetupChecklistInput, SetupItemId } from './setupChecklist'
import type { CertificateStatus } from '../queries/certificates'

function cert(domain: string, status: CertificateStatus['status']) {
  return { domain, status, not_before: '', not_after: '' } as CertificateStatus
}

const allDone: SetupChecklistInput = {
  gitProviders: [{ provider: 'github', connected: true }] as never,
  apps: [{ name: 'web' }] as never,
  domains: [{ domain: 'a.example.com', service_name: 'web' }],
  certificates: [cert('a.example.com', 'healthy')],
  backupTargets: [{ id: 't' }] as never,
  controlPlaneBackups: [{ name: 'b' }] as never,
  channels: [{ id: 'c' }] as never,
  alertRules: [{ id: 'r' }] as never,
  twoFactor: { enabled: true, recovery_codes_remaining: 8 },
  dashboardUrl: { dashboard_url: 'https://ops.example.com' },
}

function stateOf(input: SetupChecklistInput, id: SetupItemId) {
  return computeSetupChecklist(input).items.find((i) => i.id === id)?.state
}

describe('computeSetupChecklist', () => {
  const cases: [string, SetupChecklistInput, SetupItemId, string][] = [
    ['source connected', allDone, 'source', 'done'],
    [
      'source none connected',
      {
        ...allDone,
        gitProviders: [{ provider: 'github', connected: false }] as never,
      },
      'source',
      'todo',
    ],
    ['source 403', { ...allDone, gitProviders: null }, 'source', 'unavailable'],
    ['no apps', { ...allDone, apps: [] }, 'app', 'todo'],
    ['no domains', { ...allDone, domains: [] }, 'domain', 'todo'],
    [
      'domain with expired cert',
      { ...allDone, certificates: [cert('a.example.com', 'expired')] },
      'domain',
      'todo',
    ],
    [
      'domain cert for other host',
      { ...allDone, certificates: [cert('b.example.com', 'healthy')] },
      'domain',
      'todo',
    ],
    [
      'domain cert list unavailable',
      { ...allDone, certificates: null },
      'domain',
      'done',
    ],
    ['backup no targets', { ...allDone, backupTargets: [] }, 'backup', 'todo'],
    [
      'backup target but no backup',
      { ...allDone, controlPlaneBackups: [] },
      'backup',
      'todo',
    ],
    [
      'backup targets 404',
      { ...allDone, backupTargets: null },
      'backup',
      'unavailable',
    ],
    ['alerts no channel', { ...allDone, channels: [] }, 'alerts', 'todo'],
    ['alerts no rule', { ...allDone, alertRules: [] }, 'alerts', 'todo'],
    ['alerts 501', { ...allDone, alertRules: null }, 'alerts', 'unavailable'],
    [
      '2fa off',
      {
        ...allDone,
        twoFactor: { enabled: false, recovery_codes_remaining: 0 },
      },
      'twoFactor',
      'todo',
    ],
    [
      'dashboard url blank',
      { ...allDone, dashboardUrl: { dashboard_url: ' ' } },
      'dashboardUrl',
      'todo',
    ],
  ]

  it.each(cases)('%s', (_name, input, id, expected) => {
    expect(stateOf(input, id)).toBe(expected)
  })

  it('is complete when everything is done', () => {
    const c = computeSetupChecklist(allDone)
    expect(c).toMatchObject({
      done: 7,
      total: 7,
      complete: true,
      loading: false,
    })
  })

  it('excludes unavailable items from progress', () => {
    const c = computeSetupChecklist({ ...allDone, twoFactor: null })
    expect(c).toMatchObject({ done: 6, total: 6, complete: true })
  })

  it('reports loading and never completes while a query is pending', () => {
    const c = computeSetupChecklist({ ...allDone, apps: undefined })
    expect(c.loading).toBe(true)
    expect(c.complete).toBe(false)
  })

  it('counts partial progress', () => {
    const c = computeSetupChecklist({ ...allDone, apps: [], domains: [] })
    expect(c).toMatchObject({ done: 5, total: 7, complete: false })
  })
})
