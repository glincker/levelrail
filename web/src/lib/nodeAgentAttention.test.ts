import { describe, expect, it } from 'vitest'
import type { NodeResource } from '../types/nodeDetail'
import type { NodeCertState } from '../types/nodeCert'
import { nodeAgentAttentionItems } from './nodeAgentAttention'
import { buildAttentionItems, type AttentionItem } from './attention'

const node = (state?: NodeCertState, outdated = false) =>
  ({
    id: 'n1',
    name: 'web-1',
    status: 'online',
    cert: state
      ? {
          state,
          days_remaining: 5,
          generation: 1,
          key_origin: 'agent',
          warning_days: 21,
          critical_days: 7,
        }
      : undefined,
    agent: {
      outdated,
      version: 'v0.1.0',
      min_version: 'v0.2.0',
      control_plane_version: 'v0.3.0',
    },
  }) as unknown as NodeResource

describe('nodeAgentAttentionItems', () => {
  it.each([
    ['ok', []],
    ['expiring', ['warning']],
    ['critical', ['critical']],
    ['expired', ['critical']],
    ['revoked', ['warning']],
    ['unknown', []],
  ] as const)('%s certificate gives %j', (state, want) => {
    expect(nodeAgentAttentionItems(node(state)).map((i) => i.severity)).toEqual(
      want,
    )
  })

  it('flags an outdated agent', () => {
    const items = nodeAgentAttentionItems(node('ok', true))
    expect(items).toHaveLength(1)
    expect(items[0]?.title).toContain('outdated')
    expect(items[0]?.target).toEqual({ kind: 'node', id: 'n1' })
  })

  it('stays quiet for a control plane without cert data', () => {
    expect(nodeAgentAttentionItems(node())).toEqual([])
  })

  it('feeds the dashboard attention list', () => {
    const items = buildAttentionItems({ nodes: [node('expired')] })
    expect(items.map((i) => i.id)).toEqual(['node-cert:n1'])
  })
})

describe('reenroll_required nodes', () => {
  const reenrollNode = (state: NodeCertState) => {
    const n = node(state)
    n.cert = { ...n.cert!, key_origin: 'reenroll_required' }
    return n
  }

  it('leaves the reenroll item to the waiting builder and hides the renewal-failing one', () => {
    expect(nodeAgentAttentionItems(reenrollNode('critical'))).toEqual([])
    expect(nodeAgentAttentionItems(reenrollNode('revoked'))).toHaveLength(1)
  })

  it('shows the agent last renewal error on the renewal-failing item', () => {
    const n = node('expiring')
    n.cert = {
      ...n.cert!,
      renewal: {
        last_error: 'dial refused',
        next_retry_at: '2026-10-17T00:00:00Z',
        consecutive_failures: 2,
      },
    }
    const item = nodeAgentAttentionItems(n)[0] as AttentionItem
    expect(item.detail).toContain('Last error: dial refused')
    expect(item.detail).toContain('Next retry 2026-10-17T00:00:00Z')
  })
})
