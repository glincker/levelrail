import { describe, expect, it } from 'vitest'
import type { NodeResource } from '../types/nodeDetail'
import type { NodeCertState } from '../types/nodeCert'
import { nodeAgentAttentionItems } from './nodeAgentAttention'
import { buildAttentionItems } from './attention'

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
