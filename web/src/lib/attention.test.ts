import { describe, expect, it } from 'vitest'
import type { AppListEntry } from '../types/appDetail'
import type { NodeResource } from '../types/nodeDetail'
import { buildAttentionItems } from './attention'

const app = (name: string, variant: 'success' | 'destructive' | 'muted') =>
  ({ name, status: { label: 'x', variant } }) as unknown as AppListEntry
const node = (id: string, status: NodeResource['status']) =>
  ({ id, name: id, status }) as unknown as NodeResource

describe('buildAttentionItems', () => {
  it('returns nothing when everything is healthy', () => {
    expect(
      buildAttentionItems({
        apps: [app('a', 'success'), app('b', 'muted')],
        nodes: [node('n1', 'online'), node('n2', 'cordoned')],
        certs: [
          {
            domain: 'x.io',
            status: 'healthy',
            not_before: '',
            not_after: '',
          },
        ],
        doctor: {
          ok: true,
          checks: [{ code: 'c', name: 'c', status: 'ok', message: '' }],
        },
      }),
    ).toEqual([])
  })

  it('flags failing apps, offline nodes, bad certs and doctor problems, critical first', () => {
    const items = buildAttentionItems({
      apps: [app('web', 'destructive')],
      nodes: [node('n1', 'offline')],
      certs: [
        {
          domain: 'a.io',
          status: 'expiring_soon',
          not_before: '',
          not_after: '2026-10-01',
        },
        {
          domain: 'b.io',
          status: 'expired',
          not_before: '',
          not_after: '2026-09-01',
        },
      ],
      doctor: {
        ok: false,
        checks: [
          { code: 'disk', name: 'Disk', status: 'warn', message: 'low' },
          { code: 'docker', name: 'Docker', status: 'fail', message: 'down' },
          { code: 'net', name: 'Net', status: 'unknown', message: '' },
        ],
      },
    })
    expect(items.map((i) => i.id)).toEqual([
      'app:web',
      'node:n1',
      'cert:b.io',
      'doctor:docker',
      'cert:a.io',
      'doctor:disk',
    ])
    expect(items.filter((i) => i.severity === 'critical')).toHaveLength(4)
  })

  it('flags disk pressure', () => {
    const items = buildAttentionItems({
      disk: {
        level: 'critical',
        freePercent: 3,
        freeBytes: 3e9,
        reclaimableBytes: 1e9,
      },
    })
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ id: 'disk', severity: 'critical' })
  })

  it.each([
    {
      name: 'with a rollback target',
      last_good_image: 'web:1',
      error: 'build failed',
      wantDetail: 'build failed',
      wantLastGood: 'web:1',
    },
    {
      name: 'without a rollback target or error',
      last_good_image: undefined,
      error: undefined,
      wantDetail: 'No error recorded',
      wantLastGood: undefined,
    },
  ])('flags a failed deploy $name', (c) => {
    const items = buildAttentionItems({
      failedDeploys: [
        {
          id: 'dep_2',
          service_name: 'web',
          image: 'web:2',
          status: 'failed',
          started_at: new Date().toISOString(),
          error: c.error,
          last_good_image: c.last_good_image,
        },
      ],
    })
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({
      id: 'deploy:web',
      severity: 'critical',
      target: {
        kind: 'deploy',
        app: 'web',
        image: 'web:2',
        lastGoodImage: c.wantLastGood,
      },
    })
    expect(items[0].detail).toContain(c.wantDetail)
  })
})
