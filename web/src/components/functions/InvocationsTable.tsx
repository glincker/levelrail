import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  RotateCcwGlassIcon,
  ScrollGlassIcon,
} from '@thesvg/ui-react'
import { StatusPill } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { formatBytes, formatDuration } from '../../lib/functionFormat'
import {
  useFunctionInvocations,
  useReplayInvocation,
  useSetCapture,
  type FunctionInvocation,
} from '../../queries/functionInvocations'

function statusTone(status: number) {
  if (status >= 500) return 'danger'
  if (status >= 400) return 'warning'
  return 'success'
}

export function InvocationsTable({
  name,
  onReplayed,
}: Readonly<{ name: string; onReplayed?: (id: string) => void }>) {
  const { t } = useTranslation('functions')
  const query = useFunctionInvocations(name)
  const replay = useReplayInvocation(name)
  const capture = useSetCapture(name)

  const columns = useMemo<ResponsiveColumn<FunctionInvocation>[]>(
    () => [
      {
        id: 'time',
        header: t('observe.time'),
        primary: true,
        cell: (inv) => (
          <time dateTime={inv.time} className="tabular-nums">
            {new Date(inv.time).toLocaleTimeString()}
          </time>
        ),
      },
      {
        id: 'request',
        header: t('observe.request'),
        cell: (inv) => (
          <span className="font-mono text-xs">
            {inv.method} {inv.path}
          </span>
        ),
      },
      {
        id: 'status',
        header: t('observe.status'),
        cell: (inv) => (
          <StatusPill
            tone={statusTone(inv.status)}
            label={String(inv.status)}
            size="sm"
          />
        ),
      },
      {
        id: 'duration',
        header: t('observe.duration'),
        align: 'end',
        cell: (inv) => (
          <span className="tabular-nums">
            {formatDuration(inv.duration_ms)}
          </span>
        ),
      },
      {
        id: 'cold',
        header: t('observe.cold'),
        priority: 2,
        cell: (inv) => (
          <StatusPill
            tone={inv.cold_start ? 'info' : 'neutral'}
            label={inv.cold_start ? t('observe.coldYes') : t('observe.coldNo')}
            size="sm"
          />
        ),
      },
      {
        id: 'sizes',
        header: t('observe.sizes'),
        align: 'end',
        priority: 3,
        cell: (inv) => (
          <span className="tabular-nums text-muted-foreground">
            {formatBytes(inv.bytes_in)} / {formatBytes(inv.bytes_out)}
          </span>
        ),
      },
      {
        id: 'logs',
        header: t('observe.logs'),
        priority: 3,
        cell: (inv) => (
          <Link
            to="/apps/$name/logs"
            params={{ name }}
            search={{ tab: 'search', from: inv.logs_from, to: inv.logs_to }}
            className="inline-flex items-center gap-1 text-sm hover:underline"
          >
            <ScrollGlassIcon className="size-5" aria-hidden="true" />
            {t('observe.openLogs')}
          </Link>
        ),
      },
    ],
    [t, name],
  )

  const onReplay = (inv: FunctionInvocation) => {
    replay.mutate(inv.id, {
      onSuccess: (res) => {
        toast.add({
          title: t('observe.replayed', { status: res.status }),
          type: res.status >= 500 ? 'error' : 'success',
        })
        onReplayed?.(res.request_id)
      },
      onError: (err) => {
        toast.add({
          title: t('observe.replayFailed', { message: err.message }),
          type: 'error',
        })
      },
    })
  }

  let body
  if (query.isPending) {
    body = <Skeleton className="h-32 w-full" />
  } else if (query.isError) {
    body = (
      <EmptyState
        icon={
          <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
        }
        title={t('observe.loadError', { message: query.error.message })}
        description=""
        action={
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
          >
            {t('list.retry')}
          </Button>
        }
      />
    )
  } else if (query.data.invocations.length === 0) {
    body = (
      <EmptyState
        icon={<ScrollGlassIcon size="1.4em" />}
        title={t('observe.emptyTitle')}
        description={t('observe.emptyBody')}
      />
    )
  } else {
    body = (
      <ResponsiveTable
        rows={query.data.invocations}
        columns={columns}
        getRowId={(inv) => `${inv.id}-${inv.time}`}
        ariaLabel={t('observe.tableLabel', { name })}
        renderActions={(inv) =>
          inv.replayable ? (
            <Button
              variant="outline"
              size="sm"
              disabled={replay.isPending}
              onClick={() => {
                onReplay(inv)
              }}
            >
              <RotateCcwGlassIcon className="size-5" aria-hidden="true" />
              {t('observe.replay')}
            </Button>
          ) : null
        }
      />
    )
  }

  return (
    <div className="space-y-3">
      <label className="flex items-center gap-3 text-sm">
        <Switch
          checked={query.data?.capture ?? false}
          disabled={capture.isPending || query.isPending}
          onCheckedChange={(v) => {
            capture.mutate(v)
          }}
          aria-label={t('observe.captureLabel')}
        />
        <span>
          <span className="font-medium">{t('observe.captureLabel')}</span>
          <span className="block text-muted-foreground">
            {t('observe.captureHint')}
          </span>
        </span>
      </label>
      {body}
    </div>
  )
}
