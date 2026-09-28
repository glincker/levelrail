import { describe, expect, it } from 'vitest'
import type { SystemStatus } from '../../queries/systemStatus'
import type { NodeResource } from '../../types/nodeDetail'
import {
  derivePlatformIssues,
  pickBanner,
  summarizeIssues,
  type PlatformIssue,
} from './platformIssues'
import {
  isSnoozed,
  pruneExpired,
  snoozeIssue,
  snoozeUntil,
  unsnoozeIssue,
} from './snooze'

const GB = 1024 ** 3
const base: SystemStatus = {
  secrets_configured: true,
  telemetry_configured: true,
  alerts_configured: true,
  docker_connected: true,
  data_dir_total_bytes: 100 * GB,
  data_dir_free_bytes: 50 * GB,
}

const ids = (i: PlatformIssue[]) => i.map((x) => x.id)

describe('derivePlatformIssues', () => {
  it.each([
    ['healthy', base, []],
    ['disk warning', { ...base, data_dir_free_bytes: 8 * GB }, ['disk']],
    ['disk critical', { ...base, data_dir_free_bytes: 2 * GB }, ['disk']],
    [
      'docker down first',
      { ...base, docker_connected: false, data_dir_free_bytes: 2 * GB },
      ['docker-down', 'disk'],
    ],
  ])('%s', (_n, status, want) => {
    expect(ids(derivePlatformIssues({ status }))).toEqual(want)
  })

  it('adds node, cert, doctor and insecure issues with actions', () => {
    const issues = derivePlatformIssues({
      status: base,
      nodes: [{ id: 'n1', name: 'edge', status: 'offline' } as NodeResource],
      certs: [
        {
          domain: 'a.com',
          status: 'expired',
          not_after: '',
          renewal: 'stalled',
        },
      ] as never,
      doctor: {
        ok: false,
        checks: [
          { code: 'x', name: 'X', status: 'warn', message: 'm' },
          { code: 'disk_space', name: 'Disk', status: 'fail', message: 'm' },
        ],
      },
      insecure: true,
    })
    expect(ids(issues)).toEqual([
      'node-offline:n1',
      'cert:a.com',
      'doctor:disk_space',
      'doctor:x',
      'insecure-connection',
    ])
    expect(issues[0]?.actions).toEqual([
      { kind: 'link', label: 'View node', to: '/nodes/n1' },
    ])
  })

  it('drops the doctor disk check when the disk issue exists', () => {
    const issues = derivePlatformIssues({
      status: { ...base, data_dir_free_bytes: 2 * GB },
      doctor: {
        ok: false,
        checks: [
          { code: 'disk_space', name: 'D', status: 'fail', message: '' },
        ],
      },
    })
    expect(ids(issues)).toEqual(['disk'])
  })
})

describe('snooze with a fake clock', () => {
  const t0 = 1_000_000
  it.each([
    ['1h', t0 + 3_599_999, true],
    ['1h', t0 + 3_600_000, false],
    ['1d', t0 + 86_399_999, true],
    ['1d', t0 + 86_400_000, false],
    ['forever', t0 + 10 ** 12, true],
  ] as const)('%s at +%d snoozed=%s', (d, now, want) => {
    const map = snoozeIssue({}, 'a', d, t0)
    expect(isSnoozed(map, 'a', now)).toBe(want)
  })

  it('handles unknown ids, unsnooze and pruning', () => {
    const map = snoozeIssue(snoozeIssue({}, 'a', '1h', t0), 'b', 'forever', t0)
    expect(isSnoozed(map, 'zzz', t0)).toBe(false)
    expect(Object.keys(pruneExpired(map, t0 + 4_000_000))).toEqual(['b'])
    expect(isSnoozed(unsnoozeIssue(map, 'a'), 'a', t0)).toBe(false)
    expect(snoozeUntil(t0, 'forever')).toBeNull()
  })
})

describe('summarizeIssues', () => {
  const issues = derivePlatformIssues({
    status: { ...base, docker_connected: false, data_dir_free_bytes: 8 * GB },
  })

  it('reports the worst visible level', () => {
    expect(summarizeIssues(issues, () => false).level).toBe('critical')
    expect(summarizeIssues(issues, (id) => id === 'docker-down').level).toBe(
      'warning',
    )
    const all = summarizeIssues(issues, () => true)
    expect(all.level).toBe('ok')
    expect(all.snoozed).toHaveLength(2)
  })
})

describe('pickBanner never stacks', () => {
  const crit = derivePlatformIssues({
    status: { ...base, docker_connected: false, data_dir_free_bytes: 1 * GB },
  })
  const none = () => false
  it.each([
    ['everything wrong: connection wins', true, false, none, 'connection'],
    ['connection dismissed: docker', true, true, none, 'docker'],
    [
      'docker snoozed: disk',
      false,
      false,
      (id: string) => id === 'banner:docker-down',
      'disk',
    ],
    [
      'all dismissed',
      false,
      false,
      (id: string) => id.startsWith('banner:'),
      null,
    ],
  ] as const)('%s', (_n, offline, dismissed, snoozed, want) => {
    expect(
      pickBanner({
        offline,
        connectionDismissed: dismissed,
        issues: crit,
        isSnoozed: snoozed,
      }),
    ).toBe(want)
  })

  it('ignores warning-level disk', () => {
    const warn = derivePlatformIssues({
      status: { ...base, data_dir_free_bytes: 8 * GB },
    })
    expect(
      pickBanner({
        offline: false,
        connectionDismissed: false,
        issues: warn,
        isSnoozed: none,
      }),
    ).toBeNull()
  })
})
