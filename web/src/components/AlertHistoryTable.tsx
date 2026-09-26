import { Fragment, useState } from 'react'
import {
  CaretDownIcon,
  CaretRightIcon,
  ClockCounterClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { RecentChanges } from './RecentChanges'
import { useAlertHistory } from '../queries/alertNoise'
import type {
  AlertEventName,
  AlertOutcome,
  AlertHistoryEntry,
} from '../types/alertNoise'

const OUTCOME_VARIANT: Record<
  AlertOutcome,
  VariantProps<typeof badgeVariants>['variant']
> = {
  sent: 'success',
  silenced: 'muted',
  grouped: 'outline',
  inhibited: 'muted',
  failed: 'destructive',
  ratelimited: 'warning',
  flapping: 'warning',
  skipped: 'muted',
}

const OUTCOMES: AlertOutcome[] = [
  'sent',
  'silenced',
  'grouped',
  'inhibited',
  'failed',
  'ratelimited',
  'flapping',
  'skipped',
]

const EVENTS: AlertEventName[] = ['fired', 'resolved', 'flapping', 'flap_ended']

const ALL = 'all'

function HistoryRow({ entry }: { entry: AlertHistoryEntry }) {
  const [open, setOpen] = useState(false)
  const detail = [entry.detail, entry.error].filter(Boolean).join(' - ')
  const canExpand = entry.event === 'fired' && Boolean(entry.app)
  return (
    <Fragment>
      <TableRow>
        <TableCell className="whitespace-nowrap text-muted-foreground">
          {canExpand ? (
            <button
              type="button"
              aria-expanded={open}
              aria-label={`${open ? 'Hide' : 'Show'} what changed before ${entry.rule_name} fired`}
              onClick={() => {
                setOpen((v) => !v)
              }}
              className="mr-1 inline-flex size-5 items-center justify-center rounded text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
            >
              {open ? (
                <CaretDownIcon className="size-3.5" aria-hidden="true" />
              ) : (
                <CaretRightIcon className="size-3.5" aria-hidden="true" />
              )}
            </button>
          ) : null}
          {new Date(entry.at).toLocaleString()}
        </TableCell>
        <TableCell className="font-medium text-foreground">
          {entry.rule_name}
          <span className="ml-2 font-mono text-xs text-muted-foreground">
            {entry.rule_kind}
          </span>
        </TableCell>
        <TableCell className="text-muted-foreground">
          {entry.app ?? '-'}
          {entry.node ? ` on ${entry.node}` : ''}
        </TableCell>
        <TableCell>
          <Badge variant={entry.event === 'fired' ? 'destructive' : 'outline'}>
            {entry.event.replace('_', ' ')}
          </Badge>
        </TableCell>
        <TableCell>
          <Badge variant={OUTCOME_VARIANT[entry.outcome]}>
            {entry.outcome}
          </Badge>
        </TableCell>
        <TableCell className="max-w-[22rem] truncate text-xs text-muted-foreground">
          {detail || '-'}
        </TableCell>
      </TableRow>
      {open && entry.app ? (
        <TableRow>
          <TableCell colSpan={6} className="bg-muted/30 whitespace-normal">
            <RecentChanges app={entry.app} until={entry.at} />
          </TableCell>
        </TableRow>
      ) : null}
    </Fragment>
  )
}

// Alert firings and what happened to each notification. Scoped to one
// app when app is set, global otherwise.
export function AlertHistoryTable({ app }: { app?: string }) {
  const [outcome, setOutcome] = useState<AlertOutcome | ''>('')
  const [event, setEvent] = useState<AlertEventName | ''>('')
  const { data, isLoading, error } = useAlertHistory({ app, outcome, event })
  const entries = data ?? []

  return (
    <section className="space-y-3" aria-label="Alert history">
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={outcome === '' ? ALL : outcome}
          onValueChange={(v) => {
            setOutcome(v === ALL ? '' : (v as AlertOutcome))
          }}
        >
          <SelectTrigger className="w-44" aria-label="Filter by outcome">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All outcomes</SelectItem>
            {OUTCOMES.map((o) => (
              <SelectItem key={o} value={o}>
                {o}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={event === '' ? ALL : event}
          onValueChange={(v) => {
            setEvent(v === ALL ? '' : (v as AlertEventName))
          }}
        >
          <SelectTrigger className="w-44" aria-label="Filter by event">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All events</SelectItem>
            {EVENTS.map((e) => (
              <SelectItem key={e} value={e}>
                {e.replace('_', ' ')}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <TableSkeleton columnCount={6} rowCount={4} />
      ) : error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : entries.length === 0 ? (
        <EmptyState
          icon={<ClockCounterClockwiseIcon className="size-5" />}
          title="No alert history yet"
          description="Every firing, resolution and suppressed notification is recorded here."
        />
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Rule</TableHead>
                <TableHead>App</TableHead>
                <TableHead>Event</TableHead>
                <TableHead>Outcome</TableHead>
                <TableHead>Detail</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map((entry) => (
                <HistoryRow key={entry.id} entry={entry} />
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  )
}
