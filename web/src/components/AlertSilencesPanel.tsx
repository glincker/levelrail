import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { BellOffGlassIcon, CirclePlusGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import {
  useCreateSilence,
  useExpireSilence,
  useSilences,
} from '../queries/alertNoise'
import type { Severity, Silence, SilenceMatchers } from '../types/alertNoise'

const SEVERITIES: Severity[] = ['info', 'warning', 'critical']
const GO_DURATION = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/

function describeMatchers(
  m: SilenceMatchers,
  tm: TFunction<'maintenance'>,
): string {
  const parts: string[] = []
  const add = (name: string, values: string[] | undefined) => {
    if (values && values.length > 0) parts.push(`${name}: ${values.join(', ')}`)
  }
  add(tm('silence.matchApp'), m.apps)
  add(tm('silence.matchRule'), m.rule_ids)
  add(tm('silence.matchNode'), m.nodes)
  add(tm('silence.matchKind'), m.kinds)
  add(tm('silence.matchSeverity'), m.severities)
  add(tm('silence.matchMetric'), m.metrics)
  for (const [k, v] of Object.entries(m.labels ?? {})) parts.push(`${k}=${v}`)
  return parts.join(' / ')
}

function splitList(raw: string): string[] {
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

function CreateSilenceDialog() {
  const { t: tm } = useTranslation('maintenance')
  const [open, setOpen] = useState(false)
  const [apps, setApps] = useState('')
  const [nodes, setNodes] = useState('')
  const [kinds, setKinds] = useState('')
  const [severities, setSeverities] = useState<Severity[]>([])
  const [duration, setDuration] = useState('1h')
  const [reason, setReason] = useState('')
  const [formError, setFormError] = useState('')
  const create = useCreateSilence()

  function submit() {
    const matchers: SilenceMatchers = {
      apps: splitList(apps),
      nodes: splitList(nodes),
      kinds: splitList(kinds),
      severities,
    }
    if (describeMatchers(matchers, tm) === '') {
      setFormError(tm('silence.errorNoMatcher'))
      return
    }
    if (!GO_DURATION.test(duration)) {
      setFormError(tm('silence.errorDuration'))
      return
    }
    setFormError('')
    create.mutate(
      { matchers, duration, reason: reason.trim() || undefined },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: tm('silence.silencedFor', { duration }),
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" />}>
        <CirclePlusGlassIcon className="size-4.5" aria-hidden="true" />
        {tm('silence.newSilence')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{tm('silence.newSilence')}</DialogTitle>
          <DialogDescription>
            {tm('silence.dialogDescription')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Field>
            <FieldLabel htmlFor="silence-apps">{tm('silence.apps')}</FieldLabel>
            <Input
              id="silence-apps"
              placeholder={tm('silence.appsPlaceholder')}
              value={apps}
              onChange={(e) => {
                setApps(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="silence-nodes">
              {tm('silence.nodes')}
            </FieldLabel>
            <Input
              id="silence-nodes"
              placeholder={tm('silence.nodesPlaceholder')}
              value={nodes}
              onChange={(e) => {
                setNodes(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="silence-kinds">
              {tm('silence.kinds')}
            </FieldLabel>
            <Input
              id="silence-kinds"
              placeholder={tm('silence.kindsPlaceholder')}
              value={kinds}
              onChange={(e) => {
                setKinds(e.target.value)
              }}
            />
          </Field>
          <fieldset className="flex flex-wrap gap-4">
            <legend className="mb-1 text-sm font-medium">
              {tm('silence.severity')}
            </legend>
            {SEVERITIES.map((s) => (
              <label key={s} className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={severities.includes(s)}
                  onCheckedChange={(checked) => {
                    setSeverities((cur) =>
                      checked ? [...cur, s] : cur.filter((x) => x !== s),
                    )
                  }}
                />
                {tm(`silence.severityLevel.${s}`)}
              </label>
            ))}
          </fieldset>
          <Field>
            <FieldLabel htmlFor="silence-duration">
              {tm('silence.duration')}
            </FieldLabel>
            <Input
              id="silence-duration"
              value={duration}
              onChange={(e) => {
                setDuration(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="silence-reason">
              {tm('silence.reason')}
            </FieldLabel>
            <Input
              id="silence-reason"
              placeholder={tm('silence.reasonPlaceholder')}
              value={reason}
              onChange={(e) => {
                setReason(e.target.value)
              }}
            />
          </Field>
          {formError || create.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {formError || create.error?.message}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            {tm('silence.cancel')}
          </Button>
          <Button type="button" disabled={create.isPending} onClick={submit}>
            {create.isPending ? tm('silence.creating') : tm('silence.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function EndSilenceButton({ silence }: { silence: Silence }) {
  const { t: tm } = useTranslation('maintenance')
  const expire = useExpireSilence()
  if (silence.status === 'expired') return null
  if (silence.source === 'maintenance') {
    return (
      <span className="text-xs text-muted-foreground">
        {tm('silence.endsWithMaintenance', {
          domain: silence.maintenance_domain ?? '',
        })}
      </span>
    )
  }
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={expire.isPending}
      onClick={() => {
        expire.mutate(silence.id, {
          onSuccess: () => {
            toast.add({ title: tm('silence.ended'), type: 'success' })
          },
        })
      }}
    >
      {tm('silence.endNow')}
    </Button>
  )
}

export function AlertSilencesPanel() {
  const [showExpired, setShowExpired] = useState(false)
  const { data, isLoading, error } = useSilences(showExpired)
  const silences = data ?? []
  const { t } = useTranslation('common')
  const { t: tm } = useTranslation('maintenance')
  const columns = useMemo<ResponsiveColumn<Silence>[]>(
    () => [
      {
        id: 'status',
        header: tm('silence.colStatus'),
        primary: true,
        cell: (s) => (
          <Badge
            variant={
              s.status === 'active'
                ? 'warning'
                : s.status === 'pending'
                  ? 'outline'
                  : 'muted'
            }
          >
            {tm(`silence.status.${s.status}`)}
          </Badge>
        ),
      },
      {
        id: 'matches',
        header: tm('silence.colMatches'),
        cell: (s) => (
          <span className="block text-sm break-words">
            {describeMatchers(s.matchers, tm)}
            {s.reason ? (
              <span className="block text-xs text-muted-foreground">
                {s.reason}
              </span>
            ) : null}
          </span>
        ),
      },
      {
        id: 'until',
        header: tm('silence.colUntil'),
        cell: (s) => (
          <span className="text-muted-foreground">
            {new Date(s.ends_at).toLocaleString()}
          </span>
        ),
      },
      {
        id: 'createdBy',
        header: tm('silence.colCreatedBy'),
        priority: 3,
        cell: (s) => (
          <span className="text-muted-foreground">
            {s.source === 'maintenance'
              ? tm('silence.createdByMaintenance')
              : s.created_by || '-'}
          </span>
        ),
      },
    ],
    [tm],
  )

  return (
    <section className="space-y-3" aria-label={tm('silence.sectionLabel')}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <Checkbox
            checked={showExpired}
            onCheckedChange={(v) => {
              setShowExpired(v === true)
            }}
          />
          {tm('silence.showExpired')}
        </label>
        <CreateSilenceDialog />
      </div>
      {isLoading ? (
        <TableSkeleton columnCount={5} rowCount={3} />
      ) : error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : silences.length === 0 ? (
        <EmptyState
          icon={<BellOffGlassIcon className="size-5.5" />}
          title={tm('silence.emptyTitle')}
          description={tm('silence.emptyDescription')}
        />
      ) : (
        <ResponsiveTable
          rows={silences}
          columns={columns}
          getRowId={(s) => s.id}
          ariaLabel={t('tableLabels.alertSilences')}
          renderActions={(s) => <EndSilenceButton silence={s} />}
        />
      )}
    </section>
  )
}
