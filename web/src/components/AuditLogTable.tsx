import { useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { RobotIcon } from '@phosphor-icons/react/dist/ssr'
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

function VirtualAuditRow({ entry }: { entry: AuditLogEntry }) {
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
      <span role="cell">{entry.ability}</span>
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

function VirtualAuditTable({ entries }: { entries: AuditLogEntry[] }) {
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
                <VirtualAuditRow entry={entry} />
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

export function AuditLogTable({ entries }: { entries: AuditLogEntry[] }) {
  if (entries.length > AUDIT_VIRTUALIZE_OVER) {
    return <VirtualAuditTable entries={entries} />
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
              <TableCell>{entry.ability}</TableCell>
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
