import { useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { RobotIcon } from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from './ui/table'
import { Badge } from './ui/badge'
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

const GRID =
  'grid grid-cols-[10rem_minmax(0,1.4fr)_minmax(0,1fr)_5rem_5rem_4rem_minmax(0,2fr)_4rem] items-center gap-3 px-4'

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
      <RobotIcon
        className="size-3.5 text-muted-foreground"
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
      <span>{friendly.label}</span>
      {friendly.domain ? (
        <DomainTag domain={friendly.domain} domainApp={domainApp} />
      ) : null}
    </span>
  )
}

function VirtualAuditRow({
  entry,
  domainApp,
}: {
  entry: AuditLogEntry
  domainApp?: DomainAppLookup
}) {
  return (
    <div role="row" className={`${GRID} border-b border-border py-2 text-sm`}>
      <span role="cell" className="text-muted-foreground">
        {formatDate(entry.created_at)}
      </span>
      <span role="cell" className="truncate font-medium text-foreground">
        {entry.actor_name}
        <Badge variant="outline" className="ml-2">
          {entry.actor_type}
        </Badge>
      </span>
      <span role="cell" className="truncate">
        <AgentCell entry={entry} />
      </span>
      <span role="cell">
        <ClientKindBadge clientKind={entry.client_kind} />
      </span>
      <span role="cell">
        <AbilityCell entry={entry} domainApp={domainApp} />
      </span>
      <span role="cell" className="font-mono text-xs">
        {entry.method}
      </span>
      <span role="cell" className="truncate font-mono text-xs">
        {entry.path}
      </span>
      <span role="cell">
        <StatusBadge status={entry.status_code} />
      </span>
    </div>
  )
}

function VirtualAuditTable({
  entries,
  domainApp,
}: {
  entries: AuditLogEntry[]
  domainApp?: DomainAppLookup
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 45,
    overscan: 8,
  })
  return (
    <div
      role="table"
      aria-label="Audit log"
      aria-rowcount={entries.length + 1}
      className="rounded-lg border border-border"
    >
      <div
        role="row"
        className={`${GRID} border-b border-border py-2 text-xs font-medium text-muted-foreground`}
      >
        <span role="columnheader">Time</span>
        <span role="columnheader">Actor</span>
        <span role="columnheader">Agent</span>
        <span role="columnheader">Client</span>
        <span role="columnheader">Ability</span>
        <span role="columnheader">Method</span>
        <span role="columnheader">Path</span>
        <span role="columnheader">Status</span>
      </div>
      <div ref={parentRef} className="h-[60vh] overflow-auto">
        <div
          style={{ height: virtualizer.getTotalSize(), position: 'relative' }}
        >
          {virtualizer.getVirtualItems().map((v) => {
            const entry = entries[v.index]
            if (!entry) {
              return null
            }
            return (
              <div
                key={entry.id}
                data-index={v.index}
                ref={virtualizer.measureElement}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${v.start}px)`,
                }}
              >
                <VirtualAuditRow entry={entry} domainApp={domainApp} />
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

export function AuditLogTable({
  entries,
  domainApp,
}: {
  entries: AuditLogEntry[]
  domainApp?: DomainAppLookup
}) {
  if (entries.length > AUDIT_VIRTUALIZE_OVER) {
    return <VirtualAuditTable entries={entries} domainApp={domainApp} />
  }
  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Time</TableHead>
            <TableHead>Actor</TableHead>
            <TableHead>Agent</TableHead>
            <TableHead>Client</TableHead>
            <TableHead>Ability</TableHead>
            <TableHead>Method</TableHead>
            <TableHead>Path</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((entry) => (
            <TableRow key={entry.id}>
              <TableCell className="text-muted-foreground">
                {formatDate(entry.created_at)}
              </TableCell>
              <TableCell className="font-medium text-foreground">
                {entry.actor_name}
                <Badge variant="outline" className="ml-2">
                  {entry.actor_type}
                </Badge>
              </TableCell>
              <TableCell>
                <AgentCell entry={entry} />
              </TableCell>
              <TableCell>
                <ClientKindBadge clientKind={entry.client_kind} />
              </TableCell>
              <TableCell>
                <AbilityCell entry={entry} domainApp={domainApp} />
              </TableCell>
              <TableCell className="font-mono text-xs">
                {entry.method}
              </TableCell>
              <TableCell className="font-mono text-xs">{entry.path}</TableCell>
              <TableCell>
                <StatusBadge status={entry.status_code} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
