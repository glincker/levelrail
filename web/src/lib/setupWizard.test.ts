import { describe, expect, it } from 'vitest'
import {
  appGate,
  domainGate,
  domainProgress,
  firstAppPhase,
  gitGate,
  liveUrl,
  nextStep,
  normalizeDomain,
  pickTrackedApp,
  pollInterval,
  publicIpFromDoctor,
  resumeStep,
  serverCheckGate,
  withStepStatus,
} from './setupWizard'
import type { DoctorCheck, DoctorReport } from '../queries/systemDoctor'
import type { AppListEntry } from '../types/appDetail'
import type { IngressDomainCheckResult } from '../queries/domains'
import type { CertificateStatus } from '../queries/certificates'

function report(checks: Partial<DoctorCheck>[]): DoctorReport {
  return {
    ok: !checks.some((c) => c.status === 'fail'),
    checks: checks.map((c) => ({
      code: 'x',
      name: 'X',
      status: 'ok',
      message: '',
      ...c,
    })),
  }
}

function app(overrides: Partial<AppListEntry>): AppListEntry {
  return {
    name: 'sample-app',
    image: 'nginx:alpine',
    port: 80,
    bind_address: 'private',
    strategy: 'rolling',
    replicas: 1,
    status: { label: 'Reconciling', variant: 'muted' },
    ...overrides,
  } as AppListEntry
}

describe('resumeStep', () => {
  it.each([
    {
      name: 'fresh instance starts at server',
      current: '',
      steps: {},
      want: 'server',
    },
    {
      name: 'saved step wins',
      current: 'app',
      steps: { server: 'completed' as const },
      want: 'app',
    },
    {
      name: 'unknown saved step falls back to first unfinished',
      current: 'billing',
      steps: { server: 'completed' as const, domain: 'skipped' as const },
      want: 'git',
    },
    {
      name: 'everything finished lands on done',
      current: '',
      steps: {
        server: 'completed' as const,
        domain: 'skipped' as const,
        git: 'skipped' as const,
        app: 'completed' as const,
        done: 'completed' as const,
      },
      want: 'done',
    },
  ])('$name', ({ current, steps, want }) => {
    expect(resumeStep(current, steps)).toBe(want)
  })

  it('nextStep advances and stops at done', () => {
    expect(nextStep('server')).toBe('domain')
    expect(nextStep('app')).toBe('done')
    expect(nextStep('done')).toBe('done')
  })

  it('withStepStatus does not mutate its input', () => {
    const before = { server: 'completed' as const }
    const after = withStepStatus(before, 'domain', 'skipped')
    expect(before).toEqual({ server: 'completed' })
    expect(after).toEqual({ server: 'completed', domain: 'skipped' })
  })
})

describe('serverCheckGate', () => {
  it('blocks while checks are loading', () => {
    expect(serverCheckGate(undefined)).toEqual({
      canContinue: false,
      reason: 'Checks are still running.',
    })
  })

  it('blocks on an unreachable Docker daemon with its name in the reason', () => {
    const gate = serverCheckGate(
      report([{ code: 'docker', name: 'Docker daemon', status: 'fail' }]),
    )
    expect(gate.canContinue).toBe(false)
    if (!gate.canContinue) expect(gate.reason).toContain('Docker daemon')
  })

  it('allows warnings and soft failures through', () => {
    const gate = serverCheckGate(
      report([
        { code: 'docker', status: 'ok' },
        { code: 'ram', status: 'warn' },
        { code: 'port_80', status: 'fail' },
      ]),
    )
    expect(gate).toEqual({ canContinue: true })
  })
})

describe('publicIpFromDoctor', () => {
  it('reads the detected address from a passing public_ip check', () => {
    expect(
      publicIpFromDoctor(
        report([{ code: 'public_ip', message: 'detected 203.0.113.7' }]),
      ),
    ).toBe('203.0.113.7')
  })

  it('returns undefined when the check did not pass', () => {
    expect(
      publicIpFromDoctor(
        report([{ code: 'public_ip', status: 'unknown', message: 'offline' }]),
      ),
    ).toBeUndefined()
  })
})

describe('normalizeDomain', () => {
  it.each([
    ['Dash.Example.com', 'dash.example.com'],
    ['https://dash.example.com/', 'dash.example.com'],
    ['not a domain', ''],
    ['localhost', ''],
  ])('%s -> %s', (raw, want) => {
    expect(normalizeDomain(raw)).toBe(want)
  })
})

describe('domainProgress and domainGate', () => {
  const domain = 'dash.example.com'
  type ConfiguredCheck = Extract<IngressDomainCheckResult, { configured: true }>
  const checkFor = (overrides: Partial<ConfiguredCheck>): ConfiguredCheck => ({
    configured: true,
    domain,
    resolved: false,
    status: 'not_resolving',
    ...overrides,
  })
  const cert: CertificateStatus = {
    domain,
    issuer: "Let's Encrypt",
    not_before: '2026-09-01T00:00:00Z',
    not_after: '2026-12-01T00:00:00Z',
    status: 'healthy',
  }

  it('blocks before the domain is saved', () => {
    const gate = domainGate(false, undefined)
    expect(gate.canContinue).toBe(false)
  })

  it('waits on DNS and keeps the certificate pending', () => {
    const p = domainProgress({
      domain,
      check: checkFor({}),
      certificates: [],
      publicIp: undefined,
    })
    expect(p.dns).toBe('waiting')
    expect(p.cert).toBe('pending')
    const gate = domainGate(true, p)
    expect(gate.canContinue).toBe(false)
    if (!gate.canContinue) expect(gate.reason).toMatch(/^Waiting for DNS/)
  })

  it('flags a record pointing somewhere else', () => {
    const p = domainProgress({
      domain,
      check: checkFor({
        resolved: true,
        resolved_hosts: ['198.51.100.1'],
        status: 'resolves_elsewhere',
      }),
      certificates: [],
      publicIp: '203.0.113.7',
    })
    expect(p.dns).toBe('error')
    expect(p.dnsDetail).toContain('198.51.100.1')
  })

  it('treats a match on the detected public IP as resolved even when the check guessed a private host', () => {
    const p = domainProgress({
      domain,
      check: checkFor({
        resolved: true,
        resolved_hosts: ['203.0.113.7'],
        status: 'resolves_elsewhere',
      }),
      certificates: [],
      publicIp: '203.0.113.7',
    })
    expect(p.dns).toBe('ok')
    expect(p.cert).toBe('waiting')
    const gate = domainGate(true, p)
    if (!gate.canContinue) expect(gate.reason).toMatch(/^Waiting for HTTPS/)
  })

  it('opens once the certificate exists', () => {
    const p = domainProgress({
      domain,
      check: checkFor({ resolved: true, status: 'connected' }),
      certificates: [cert],
      publicIp: undefined,
    })
    expect(p.cert).toBe('ok')
    expect(domainGate(true, p)).toEqual({ canContinue: true })
  })

  it('matches a certificate by SAN and rejects an expired one', () => {
    const p = domainProgress({
      domain,
      check: checkFor({ resolved: true, status: 'connected' }),
      certificates: [
        {
          ...cert,
          domain: 'other.example.com',
          sans: [domain],
          status: 'expired',
        },
      ],
      publicIp: undefined,
    })
    expect(p.cert).toBe('error')
  })

  it('ignores a check result for a previously saved domain', () => {
    const p = domainProgress({
      domain,
      check: {
        ...checkFor({ status: 'connected', resolved: true }),
        domain: 'old.example.com',
      },
      certificates: [cert],
      publicIp: undefined,
    })
    expect(p.dns).toBe('waiting')
  })
})

describe('pollInterval', () => {
  it.each([
    { name: 'polls while waiting', done: false, elapsed: 1_000, want: 5_000 },
    { name: 'stops when done', done: true, elapsed: 1_000, want: false },
    {
      name: 'stops after the time budget',
      done: false,
      elapsed: 60_000,
      want: false,
    },
  ])('$name', ({ done, elapsed, want }) => {
    expect(pollInterval(done, 0, elapsed, 5_000, 60_000)).toBe(want)
  })
})

describe('gitGate', () => {
  it('requires one connected provider', () => {
    const base = {
      can_list_branches: true,
      can_register_webhook: true,
      can_auth_clone: true,
    }
    expect(
      gitGate([{ ...base, provider: 'github', connected: false }]).canContinue,
    ).toBe(false)
    expect(gitGate([{ ...base, provider: 'gitea', connected: true }])).toEqual({
      canContinue: true,
    })
  })
})

describe('first app state machine', () => {
  it('prefers the sample app, else the first app', () => {
    const apps = [app({ name: 'api' }), app({ name: 'sample-app' })]
    expect(pickTrackedApp(apps, 'sample-app')?.name).toBe('sample-app')
    expect(pickTrackedApp([app({ name: 'api' })], 'sample-app')?.name).toBe(
      'api',
    )
    expect(pickTrackedApp([], 'sample-app')).toBeUndefined()
  })

  it.each([
    { name: 'no app yet', a: undefined, elapsed: 0, want: 'none' },
    { name: 'reconciling', a: app({}), elapsed: 1_000, want: 'deploying' },
    {
      name: 'reconciling too long',
      a: app({}),
      elapsed: 200_000,
      want: 'slow',
    },
    {
      name: 'healthy',
      a: app({ status: { label: 'Healthy', variant: 'success' } }),
      elapsed: 0,
      want: 'live',
    },
    {
      name: 'attention needed',
      a: app({ status: { label: 'Attention needed', variant: 'destructive' } }),
      elapsed: 0,
      want: 'failed',
    },
  ])('$name -> $want', ({ a, elapsed, want }) => {
    expect(firstAppPhase(a, 0, elapsed, 120_000)).toBe(want)
  })

  it('only lets Continue through once live', () => {
    expect(appGate('live')).toEqual({ canContinue: true })
    for (const phase of ['none', 'deploying', 'slow', 'failed'] as const) {
      expect(appGate(phase).canContinue).toBe(false)
    }
  })

  it('picks a domain, then the fallback URL, then a reachable host port', () => {
    expect(liveUrl(app({ domains: ['a.example.com'] }), undefined, 'x')).toBe(
      'https://a.example.com',
    )
    expect(
      liveUrl(
        app({}),
        {
          container_port: 80,
          running: true,
          fallback_url: 'https://s.sslip.io',
        },
        'x',
      ),
    ).toBe('https://s.sslip.io')
    expect(
      liveUrl(
        app({}),
        { container_port: 80, host_port: 32768, running: true },
        'localhost',
      ),
    ).toBe('http://localhost:32768')
    expect(
      liveUrl(
        app({}),
        { container_port: 80, host_port: 32768, running: true },
        '203.0.113.7',
      ),
    ).toBeUndefined()
  })
})
