import type { Icon } from '@phosphor-icons/react'
import {
  ShieldCheckIcon,
  ShieldIcon,
  ShieldWarningIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { AuditLogEntry } from '../queries/auditLog'
import type auditLogEn from '../locales/en/auditLog.json'

// Derived from the real auditLog.json's own "labels" keys, not
// hand-duplicated, so a renamed/removed JSON key is a tsc error here
// too, the same "JSON file is the schema" guarantee docs/i18n.md
// describes for resources.ts.
type AuditLabelKey = `labels.${keyof typeof auditLogEn.labels}`

export interface AuditFriendlyLabel {
  icon: Icon
  // labelKey is an auditLog namespace key, translated by the caller
  // (AuditLogTable's useTranslation('auditLog')); this module has no
  // JSX and renders no text itself.
  labelKey: AuditLabelKey
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
  labelKey: AuditLabelKey
  matches: (
    entry: Pick<AuditLogEntry, 'ability' | 'method' | 'path' | 'action'>,
  ) => boolean
}

const AUDIT_LABEL_RULES: AuditLabelRule[] = [
  {
    icon: TerminalWindowIcon,
    labelKey: 'labels.deviceLoginApproved',
    matches: (e) => e.action === 'device_login.approved',
  },
  {
    icon: TerminalWindowIcon,
    labelKey: 'labels.deviceLoginDenied',
    matches: (e) => e.action === 'device_login.denied',
  },
  {
    icon: TerminalWindowIcon,
    labelKey: 'labels.deviceLoginExpired',
    matches: (e) => e.action === 'device_login.expired',
  },
  {
    icon: TerminalWindowIcon,
    labelKey: 'labels.deviceLoginDismissed',
    matches: (e) => e.action === 'device_login.dismissed',
  },
  {
    icon: ShieldCheckIcon,
    labelKey: 'labels.certIssued',
    matches: (e) => e.ability === 'cert.issued',
  },
  {
    icon: ShieldCheckIcon,
    labelKey: 'labels.certRenewed',
    matches: (e) => e.ability === 'cert.renewed',
  },
  {
    icon: ShieldIcon,
    labelKey: 'labels.certRenewalRequested',
    matches: (e) => e.method === 'POST' && e.path.endsWith('/cert/renew'),
  },
  {
    icon: ShieldCheckIcon,
    labelKey: 'labels.certUploaded',
    matches: (e) => e.method === 'PUT' && e.path.endsWith('/tls-cert'),
  },
  {
    icon: ShieldWarningIcon,
    labelKey: 'labels.certRemoved',
    matches: (e) => e.method === 'DELETE' && e.path.endsWith('/tls-cert'),
  },
]

// auditFriendlyLabel maps one audit_log entry to a specific label key
// and domain tag, the same idea Nginx Proxy Manager's own audit log
// applies to its certificate rows ("Renewed Certificate", the domain as
// a tag) rather than this page's previous raw ability/method/path
// columns for every row. Returns null when no rule matches, so callers
// fall back to the existing raw columns unchanged.
export function auditFriendlyLabel(
  entry: Pick<AuditLogEntry, 'ability' | 'method' | 'path' | 'action'>,
): AuditFriendlyLabel | null {
  const rule = AUDIT_LABEL_RULES.find((r) => r.matches(entry))
  if (!rule) {
    return null
  }
  return {
    icon: rule.icon,
    labelKey: rule.labelKey,
    domain: domainFromPath(entry.path),
  }
}
