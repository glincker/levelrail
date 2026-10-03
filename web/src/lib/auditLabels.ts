import type { Icon } from '@phosphor-icons/react'
import {
  ShieldCheckIcon,
  ShieldIcon,
  ShieldWarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { AuditLogEntry } from '../queries/auditLog'

export interface AuditFriendlyLabel {
  icon: Icon
  label: string
  // domain is set whenever path carries one, even if no rule below
  // matched: callers (AuditLogTable) still want it for the "could not
  // be read" fallback case the task this file was built for calls out.
  domain?: string
}

// DOMAIN_IN_PATH_RE matches both request-driven paths
// (.../domains/{domain}/cert/renew, .../domains/{domain}/tls-cert) and
// the synthetic system-event path internal/ingress/certstorage.go's
// certEventPath builds (/api/v1/certificates/{domain}): one capture
// group, applied in order, first match wins.
const DOMAIN_IN_PATH_RE = /\/domains\/([^/]+)\//
const CERT_EVENT_PATH_RE = /\/certificates\/([^/]+)$/

function domainFromPath(path: string): string | undefined {
  const match = DOMAIN_IN_PATH_RE.exec(path) ?? CERT_EVENT_PATH_RE.exec(path)
  const raw = match?.[1]
  if (!raw) {
    return undefined
  }
  try {
    return decodeURIComponent(raw)
  } catch {
    return raw
  }
}

// AuditLabelRule is one (ability, method, path-suffix) -> friendly label
// mapping. A small ordered list rather than a class hierarchy: this
// mirrors certStatus.ts's own "small lookup plus a couple of pure
// functions" shape, general enough to add a non-certificate row later
// (e.g. a deploy rollback) without restructuring anything.
interface AuditLabelRule {
  icon: Icon
  label: string
  matches: (
    entry: Pick<AuditLogEntry, 'ability' | 'method' | 'path'>,
  ) => boolean
}

const AUDIT_LABEL_RULES: AuditLabelRule[] = [
  {
    icon: ShieldCheckIcon,
    label: 'Issued Certificate',
    matches: (e) => e.ability === 'cert.issued',
  },
  {
    icon: ShieldCheckIcon,
    label: 'Renewed Certificate',
    matches: (e) => e.ability === 'cert.renewed',
  },
  {
    icon: ShieldIcon,
    label: 'Requested Certificate Renewal',
    matches: (e) => e.method === 'POST' && e.path.endsWith('/cert/renew'),
  },
  {
    icon: ShieldCheckIcon,
    label: 'Uploaded Certificate',
    matches: (e) => e.method === 'PUT' && e.path.endsWith('/tls-cert'),
  },
  {
    icon: ShieldWarningIcon,
    label: 'Removed Certificate',
    matches: (e) => e.method === 'DELETE' && e.path.endsWith('/tls-cert'),
  },
]

// auditFriendlyLabel maps one audit_log entry to a specific,
// human-readable label and domain tag, the same idea Nginx Proxy
// Manager's own audit log applies to its certificate rows ("Renewed
// Certificate", the domain as a tag) rather than this page's previous
// raw ability/method/path columns for every row regardless of what it
// represents. Returns null when no rule matches, so callers fall back to
// the existing raw columns unchanged.
export function auditFriendlyLabel(
  entry: Pick<AuditLogEntry, 'ability' | 'method' | 'path'>,
): AuditFriendlyLabel | null {
  const rule = AUDIT_LABEL_RULES.find((r) => r.matches(entry))
  if (!rule) {
    return null
  }
  return {
    icon: rule.icon,
    label: rule.label,
    domain: domainFromPath(entry.path),
  }
}
