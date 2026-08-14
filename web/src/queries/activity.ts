// Data source for the notification bell (components/NotificationBell.tsx):
// a bounded v1 activity feed built entirely from two endpoints that are
// already global and already cheap, GET /api/v1/nodes and
// GET /api/v1/certificates, both a single request each, no fan-out.
//
// Deliberately NOT included, and this is a real scope decision, not an
// oversight: failed deploys and alert-rule firings. Both would be the
// obviously-useful third and fourth signal for an operator glancing at
// this bell, but neither has a global endpoint to read them from.
// GET /api/v1/apps/{name}/deploys (queries/deploys.ts) returns one app's
// current reconcile conditions, not a cross-app history; GET
// /api/v1/apps/{name}/alerts (queries/alerts.ts) is the same shape, one
// app's rules. Building either into this bell honestly would mean
// fetching every app's own sub-resource on an interval, an N+1 fan-out
// this project's own frontend rule explicitly warns against ("never
// fetch the full resource graph on page load", CLAUDE.md's frontend
// section) and exactly the kind of always-on background cost section
// 4.8 calls out as what makes competitors "heavy." Closing this gap for
// real is a backend job (a real aggregation endpoint, or a deploy
// history table per section 1.9's own TODO), not something this pass
// fakes by polling every app from the browser instead.
//
// nodes.status and certificates.status already carry the two signals
// this bell surfaces, no derived/inferred state: a node's `status` field
// answers "is it offline" outright (types/nodeDetail.ts), and
// `schedulable` answers "is it cordoned" the same documented way
// NodeRow.tsx/CordonNodeDialog.tsx already drive their own display off
// it. CertificateStatus.status answers "expiring or expired" the same
// direct way routes/settings/general.tsx's own CertificatesCard reads
// it. This module adds no new backend surface, it only re-reads two
// existing ones on an interval and reshapes them into one flat list.

import { useQuery } from '@tanstack/react-query'
import { nodeListQueryOptions } from './nodes'
import {
  certificatesQueryOptions,
  type CertificateStatus,
} from './certificates'
import type { NodeResource } from '../types/nodeDetail'

export type ActivitySeverity = 'critical' | 'warning'

// kind exists purely so the presentation layer (NotificationBell.tsx) can
// pick an icon without string-matching `id`: id itself is only a stable
// React key plus a debugging label, never parsed.
export type ActivityKind =
  'node_offline' | 'node_cordoned' | 'cert_expired' | 'cert_expiring'

export interface ActivityEvent {
  id: string
  kind: ActivityKind
  severity: ActivitySeverity
  title: string
  detail: string
  href: string
}

// 30s: frequent enough that a node going offline or a cert crossing into
// "expiring soon" shows up on the next glance without a manual refresh,
// not so frequent it reads as the kind of always-on polling section 4.8
// singles out. Scoped to this hook's own two queries only, nothing else
// in the app inherits it.
const ACTIVITY_POLL_INTERVAL_MS = 30_000

function nodeEvents(nodes: NodeResource[]): ActivityEvent[] {
  const events: ActivityEvent[] = []
  for (const node of nodes) {
    // Driven off `status === 'offline'` and `schedulable`, the same two
    // independent, documented fields NodeRow.tsx and CordonNodeDialog.tsx
    // already use, never a derived "cordoned" status value (nothing sets
    // one, see types/nodeDetail.ts's own NodeStatus comment).
    if (node.status === 'offline') {
      events.push({
        id: `node-offline-${node.id}`,
        kind: 'node_offline',
        severity: 'critical',
        title: `${node.name} is offline`,
        detail: node.last_seen_at
          ? `Last seen ${new Date(node.last_seen_at).toLocaleString()}`
          : 'Never connected',
        href: `/nodes/${node.id}`,
      })
    }
    if (!node.schedulable) {
      events.push({
        id: `node-cordoned-${node.id}`,
        kind: 'node_cordoned',
        severity: 'warning',
        title: `${node.name} is cordoned`,
        detail: 'Not accepting new app or database placements',
        href: `/nodes/${node.id}`,
      })
    }
  }
  return events
}

function certificateEvents(certs: CertificateStatus[]): ActivityEvent[] {
  const events: ActivityEvent[] = []
  for (const cert of certs) {
    const notAfter = new Date(cert.not_after).toLocaleDateString()
    if (cert.status === 'expired') {
      events.push({
        id: `cert-expired-${cert.domain}`,
        kind: 'cert_expired',
        severity: 'critical',
        title: `Certificate for ${cert.domain} expired`,
        detail: `Expired ${notAfter}`,
        href: '/settings/general',
      })
    } else if (cert.status === 'expiring_soon') {
      events.push({
        id: `cert-expiring-${cert.domain}`,
        kind: 'cert_expiring',
        severity: 'warning',
        title: `Certificate for ${cert.domain} is expiring soon`,
        detail: `Expires ${notAfter}`,
        href: '/settings/general',
      })
    }
  }
  return events
}

// Plain useQuery, not suspense: this bell renders in every authenticated
// route's header (routes/__root.tsx), so it must degrade the same
// graceful way useNodeListOptional's own doc comment establishes for an
// optional signal, a failure on either source (a 403 under some future
// scoped token, a transient network error) must never take down the
// header it lives in, retry: false so a real outage doesn't hammer
// either endpoint every render either.
export function useActivityEvents(): {
  events: ActivityEvent[]
  isLoading: boolean
} {
  const nodes = useQuery({
    ...nodeListQueryOptions(),
    retry: false,
    refetchInterval: ACTIVITY_POLL_INTERVAL_MS,
  })
  const certificates = useQuery({
    ...certificatesQueryOptions(),
    retry: false,
    refetchInterval: ACTIVITY_POLL_INTERVAL_MS,
  })

  const events = [
    ...nodeEvents(nodes.data ?? []),
    ...certificateEvents(certificates.data ?? []),
  ].sort((a, b) => {
    if (a.severity === b.severity) return 0
    return a.severity === 'critical' ? -1 : 1
  })

  return {
    events,
    isLoading: nodes.isLoading || certificates.isLoading,
  }
}
