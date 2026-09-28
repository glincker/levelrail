import type { AttentionItem } from './attention'
import type { NodeResource } from '../types/nodeDetail'
import { nodeCertLabel } from './nodeCertLabel'

// Mirrors internal/attention's nodeAgentItems: failing certificate
// renewal, expired or revoked certificates, and outdated agents.
export function nodeAgentAttentionItems(node: NodeResource): AttentionItem[] {
  const items: AttentionItem[] = []
  const target = { kind: 'node' as const, id: node.id }
  const cert = node.cert
  if (cert) {
    const renewalFailing = 'renewal is not succeeding'
    switch (cert.state) {
      case 'expired':
        items.push({
          id: `node-cert:${node.id}`,
          severity: 'critical',
          title: `Node ${node.name}: agent certificate expired`,
          detail: 'Re-enroll the node from its detail page',
          target,
        })
        break
      case 'critical':
      case 'expiring':
        items.push({
          id: `node-cert:${node.id}`,
          severity: cert.state === 'critical' ? 'critical' : 'warning',
          title: `Node ${node.name}: ${nodeCertLabel(cert).toLowerCase()}`,
          detail: `Certificate ${renewalFailing}`,
          target,
        })
        break
      case 'revoked':
        items.push({
          id: `node-cert:${node.id}`,
          severity: 'warning',
          title: `Node ${node.name}: agent certificate revoked`,
          detail: 'Re-enroll or delete the node',
          target,
        })
        break
    }
  }
  if (node.agent?.outdated) {
    items.push({
      id: `node-agent:${node.id}`,
      severity: 'warning',
      title: `Node ${node.name}: agent is outdated`,
      detail: `Agent ${node.agent.version || 'version unknown'} is older than the minimum ${node.agent.min_version ?? ''}`,
      target,
    })
  }
  return items
}
