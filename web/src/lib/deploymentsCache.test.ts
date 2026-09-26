import { describe, expect, it } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { makeDeployment } from '../test/deploymentFixtures'
import { EMPTY_FILTERS } from './deploymentFilters'
import {
  applyEventToLane,
  applyEventToPages,
  flattenPages,
  prependToPages,
  type DeploymentPages,
} from './deploymentsCache'
import {
  parseDeploymentEvent,
  patchCaches,
} from '../hooks/useDeploymentsStream'
import { deploymentKeys } from '../queries/deployments'
import type { DeploymentEvent } from '../types/deployment'

function pages(
  ...items: ReturnType<typeof makeDeployment>[][]
): DeploymentPages {
  return {
    pages: items.map((i) => ({ items: i, next_cursor: '' })),
    pageParams: items.map(() => ''),
  }
}

function ev(
  type: DeploymentEvent['type'],
  over: Parameters<typeof makeDeployment>[0],
): DeploymentEvent {
  return { type, deployment: makeDeployment(over) }
}

describe('applyEventToPages', () => {
  it('replaces a loaded row in place without adding anything', () => {
    const data = pages([
      makeDeployment({ id: 'a', status: 'building', duration_ms: null }),
    ])
    const out = applyEventToPages(
      data,
      ev('finished', { id: 'a', status: 'ready' }),
      EMPTY_FILTERS,
      false,
    )
    expect(flattenPages(out.data).map((r) => r.status)).toEqual(['ready'])
    expect(out.pending).toBeNull()
  })

  it('holds a new matching row back when scrolled away from the top', () => {
    const data = pages([makeDeployment({ id: 'a' })])
    const out = applyEventToPages(
      data,
      ev('created', { id: 'b' }),
      EMPTY_FILTERS,
      false,
    )
    expect(flattenPages(out.data)).toHaveLength(1)
    expect(out.pending?.id).toBe('b')
  })

  it('prepends a new row when already at the top', () => {
    const data = pages([makeDeployment({ id: 'a' })])
    const out = applyEventToPages(
      data,
      ev('created', { id: 'b' }),
      EMPTY_FILTERS,
      true,
    )
    expect(flattenPages(out.data).map((r) => r.id)).toEqual(['b', 'a'])
    expect(out.pending).toBeNull()
  })

  it('ignores a new row that does not match the active filters', () => {
    const data = pages([makeDeployment({ id: 'a' })])
    const out = applyEventToPages(
      data,
      ev('created', { id: 'b', status: 'ready' }),
      { ...EMPTY_FILTERS, status: ['failed'] },
      true,
    )
    expect(out.pending).toBeNull()
    expect(flattenPages(out.data)).toHaveLength(1)
  })

  it('keeps a row that stops matching until the next refresh', () => {
    const data = pages([makeDeployment({ id: 'a', status: 'building' })])
    const out = applyEventToPages(
      data,
      ev('finished', { id: 'a', status: 'ready' }),
      { ...EMPTY_FILTERS, status: ['building'] },
      true,
    )
    expect(flattenPages(out.data).map((r) => r.id)).toEqual(['a'])
  })

  it('moves the live flag to the newest release of the same app and environment', () => {
    const data = pages([makeDeployment({ id: 'old', is_live: true })])
    const out = applyEventToPages(
      data,
      ev('finished', { id: 'new', is_live: true }),
      EMPTY_FILTERS,
      true,
    )
    const rows = flattenPages(out.data)
    expect(rows.find((r) => r.id === 'old')?.is_live).toBe(false)
    expect(rows.find((r) => r.id === 'new')?.is_live).toBe(true)
  })

  it('does not duplicate rows on prepend', () => {
    const data = pages([makeDeployment({ id: 'a' })])
    expect(
      flattenPages(prependToPages(data, [makeDeployment({ id: 'a' })])),
    ).toHaveLength(1)
  })
})

describe('applyEventToLane', () => {
  it('adds in-progress rows newest first and drops finished ones', () => {
    const a = makeDeployment({
      id: 'a',
      status: 'building',
      started_at: '2026-09-26T10:00:00Z',
    })
    let rows = applyEventToLane(
      [a],
      ev('created', {
        id: 'b',
        status: 'queued',
        started_at: '2026-09-26T10:05:00Z',
      }),
    )
    expect(rows.map((r) => r.id)).toEqual(['b', 'a'])
    rows = applyEventToLane(rows, ev('finished', { id: 'a', status: 'ready' }))
    expect(rows.map((r) => r.id)).toEqual(['b'])
  })
})

describe('stream helpers', () => {
  it('parses a valid event and rejects garbage', () => {
    const raw = JSON.stringify(ev('created', { id: 'x' }))
    expect(parseDeploymentEvent(raw)?.deployment.id).toBe('x')
    expect(parseDeploymentEvent('not json')).toBeNull()
    expect(
      parseDeploymentEvent('{"type":"nope","deployment":{"id":"x"}}'),
    ).toBeNull()
  })

  it('patches the current list, other cached lists and the lane in one pass', () => {
    const qc = new QueryClient()
    const current = deploymentKeys.list(EMPTY_FILTERS)
    const other = deploymentKeys.list({ ...EMPTY_FILTERS, app: 'web' })
    const row = makeDeployment({
      id: 'a',
      status: 'building',
      duration_ms: null,
    })
    qc.setQueryData(current, pages([row]))
    qc.setQueryData(other, pages([row]))
    qc.setQueryData(deploymentKeys.lane(), [row])

    const held = patchCaches(
      qc,
      ev('created', { id: 'n', status: 'building' }),
      EMPTY_FILTERS,
      current,
      false,
    )
    expect(held?.id).toBe('n')
    patchCaches(
      qc,
      ev('finished', { id: 'a', status: 'ready' }),
      EMPTY_FILTERS,
      current,
      false,
    )

    const cur = qc.getQueryData<DeploymentPages>(current)
    const oth = qc.getQueryData<DeploymentPages>(other)
    expect(flattenPages(cur)[0]?.status).toBe('ready')
    expect(flattenPages(oth)[0]?.status).toBe('ready')
    expect(qc.getQueryData<unknown[]>(deploymentKeys.lane())?.length).toBe(1)
  })
})
