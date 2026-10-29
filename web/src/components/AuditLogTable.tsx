import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { BotGlassIcon } from '@thesvg/ui-react'
import { Link } from '@tanstack/react-router'
import { Badge } from './ui/badge'
import { ResponsiveTable, type ResponsiveColumn } from './ui/responsive-table'
import { CLIENT_KIND_LABELS, type AuditLogEntry } from '../queries/auditLog'
import { auditFriendlyLabel } from '../lib/auditLabels'

// domainApp resolves a domain tag to its owning app, for a link to that
// app's own domains page (settings/certificates.tsx and
// CertificateCenterTable.tsx already treat a domain owned by no current
// app as an orphaned row to display plainly rather than link, the same
// fallback DomainTag below applies).
export type DomainAppLookup = Map<string, string>

function DomainTag({
  domain,
  domainApp,
}: {
  domain: string
  domainApp?: DomainAppLookup
}) {
  const appName = domainApp?.get(domain)
  if (!appName) {
    return (
      <span className="font-mono text-xs text-muted-foreground">{domain}</span>
    )
  }
  return (
    <Link
      to="/apps/$name/domains"
      params={{ name: appName }}
      className="font-mono text-xs text-foreground underline-offset-2 hover:underline"
    >
      {domain}
    </Link>
  )
}

export const AUDIT_VIRTUALIZE_OVER = 50

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

function StatusBadge({ status }: { status: number }) {
  const ok = status >= 200 && status < 300
  return <Badge variant={ok ? 'success' : 'destructive'}>{status}</Badge>
}

function ClientKindBadge({ clientKind }: { clientKind: string }) {
  return (
    <Badge variant="outline">
      {CLIENT_KIND_LABELS[clientKind] ?? clientKind}
    </Badge>
  )
}

function AgentCell({ entry }: { entry: AuditLogEntry }) {
  if (!entry.agent_name) {
    return <span className="text-muted-foreground">None</span>
  }
  return (
    <span
      className="inline-flex items-center gap-1.5 text-foreground"
      title={entry.agent_client}
    >
      <BotGlassIcon
        className="size-4 text-muted-foreground"
        aria-hidden="true"
      />
      {entry.agent_name}
    </span>
  )
}

// AbilityCell shows a specific, human-readable label and domain tag for
// an entry auditFriendlyLabel recognizes (certificate issuance, renewal,
// upload, removal); every other entry keeps the raw ability string it
// always showed, so an ability with no mapping yet is never hidden.
function AbilityCell({
  entry,
  domainApp,
}: {
  entry: AuditLogEntry
  domainApp?: DomainAppLookup
}) {
  const { t } = useTranslation('auditLog')
  const friendly = auditFriendlyLabel(entry)
  if (!friendly) {
    return <span>{entry.ability}</span>
  }
  const FriendlyIcon = friendly.icon
  return (
    <span className="inline-flex flex-wrap items-center gap-1.5">
      <FriendlyIcon
        className="size-3.5 shrink-0 text-muted-foreground"
        aria-hidden="true"
      />
      <span>{t(friendly.labelKey)}</span>
      {friendly.domain ? (
        <DomainTag domain={friendly.domain} domainApp={domainApp} />
      ) : null}
    </span>
  )
}

export function AuditLogTable({
  entries,
  domainApp,
}: {
  entries: AuditLogEntry[]
  domainApp?: DomainAppLookup
}) {
  const { t } = useTranslation('auditLog')
  const columns = useMemo<ResponsiveColumn<AuditLogEntry>[]>(
    () => [
      {
        id: 'actor',
        primary: true,
        header: t('table.actor'),
        cell: (e) => (
          <span className="font-medium text-foreground">
            {e.actor_name}
            <Badge variant="outline" className="ml-2">
              {e.actor_type}
            </Badge>
          </span>
        ),
      },
      {
        id: 'ability',
        header: t('table.ability'),
        cell: (e) => <AbilityCell entry={e} domainApp={domainApp} />,
      },
      {
        id: 'status',
        header: t('table.status'),
        width: 'w-20',
        cell: (e) => <StatusBadge status={e.status_code} />,
      },
      {
        id: 'time',
        header: t('table.time'),
        priority: 2,
        width: 'w-44',
        cell: (e) => (
          <span className="text-muted-foreground">
            {formatDate(e.created_at)}
          </span>
        ),
      },
      {
        id: 'client',
        header: t('table.client'),
        priority: 2,
        width: 'w-24',
        cell: (e) => <ClientKindBadge clientKind={e.client_kind} />,
      },
      {
        id: 'method',
        header: t('table.method'),
        priority: 3,
        width: 'w-20',
        cell: (e) => <span className="font-mono text-xs">{e.method}</span>,
      },
      {
        id: 'path',
        header: t('table.path'),
        priority: 3,
        cell: (e) => (
          <span className="block truncate font-mono text-xs" title={e.path}>
            {e.path}
          </span>
        ),
      },
      {
        id: 'agent',
        header: t('table.agent'),
        priority: 3,
        width: 'w-32',
        cell: (e) => <AgentCell entry={e} />,
      },
    ],
    [t, domainApp],
  )
  return (
    <ResponsiveTable
      rows={entries}
      columns={columns}
      getRowId={(e) => e.id}
      ariaLabel={t('table.label')}
      virtualizeOver={AUDIT_VIRTUALIZE_OVER}
      scrollHeightClass="h-[60vh]"
    />
  )
}
