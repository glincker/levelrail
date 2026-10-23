import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PaperPlaneTiltIcon,
  PencilSimpleIcon,
  TrashIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { BrandTile } from '../BrandLogoBadge'
import { formatRelativeTime } from '../../lib/relativeTime'
import {
  chipState,
  type TrafficExportChipState,
} from '../../lib/trafficExportState'
import {
  useDeleteTrafficExportSink,
  useTestTrafficExportSink,
  useUpdateTrafficExportSink,
} from '../../queries/trafficExport'
import type {
  TrafficExportSink,
  TrafficExportSinkStatus,
} from '../../types/trafficExport'

const CHIP_VARIANT = {
  active: 'success',
  idle: 'muted',
  disabled: 'muted',
  invalid: 'warning',
  failing: 'destructive',
} as const satisfies Record<TrafficExportChipState, string>

const BRAND_MARK: Partial<Record<TrafficExportSink['kind'], string>> = {
  posthog: 'posthog',
  otlp: 'opentelemetry',
}

interface Props {
  sink: TrafficExportSink
  status?: TrafficExportSinkStatus
  onEdit: (sink: TrafficExportSink) => void
}

/** TrafficExportRow is one destination: identity, status chips and the row actions on a single line. */
export function TrafficExportRow({ sink, status, onEdit }: Props) {
  const { t } = useTranslation('trafficExport')
  const update = useUpdateTrafficExportSink()
  const test = useTestTrafficExportSink()
  const remove = useDeleteTrafficExportSink()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const state = chipState(sink, status)
  const mark = BRAND_MARK[sink.kind]
  const lastSuccess =
    status && status.last_success && !status.last_success.startsWith('0001')
      ? formatRelativeTime(status.last_success)
      : ''

  function runTest() {
    test.mutate(sink.id, {
      onSuccess: (res) => {
        toast.add({
          title: res.ok
            ? t('row.testOk', { name: sink.name })
            : t('row.testFailed', {
                name: sink.name,
                reason: t(`errorKinds.${res.error_kind ?? 'other'}`, {
                  defaultValue: t('errorKinds.other'),
                }),
              }),
          type: res.ok ? 'success' : 'error',
        })
      },
      onError: (err) => {
        toast.add({
          title: t('row.testFailed', { name: sink.name, reason: err.message }),
          type: 'error',
        })
      },
    })
  }

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
      {mark ? (
        <BrandTile name={mark} label={t(`kinds.${sink.kind}`)} />
      ) : (
        <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <WebhooksLogoIcon className="size-4" aria-hidden="true" />
        </span>
      )}
      <div className="min-w-40 flex-1">
        <p className="truncate text-sm font-medium">{sink.name}</p>
        <p className="truncate text-xs text-muted-foreground">
          {t(`kinds.${sink.kind}`)}
          {sink.target_host
            ? ` · ${t('row.target', { host: sink.target_host })}`
            : ''}
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <Badge variant={CHIP_VARIANT[state]}>{t(`state.${state}`)}</Badge>
        {status?.last_error_kind ? (
          <Badge variant="destructive">
            {t(`errorKinds.${status.last_error_kind}`, {
              defaultValue: t('errorKinds.other'),
            })}
          </Badge>
        ) : null}
        <Badge variant="outline">
          {lastSuccess
            ? t('row.lastSuccess', { when: lastSuccess })
            : t('row.never')}
        </Badge>
        {status && status.queued > 0 ? (
          <Badge variant="info">
            {t('row.queued', { count: status.queued })}
          </Badge>
        ) : null}
        {status && status.dropped > 0 ? (
          <Badge variant="warning">
            {t('row.dropped', { count: status.dropped })}
          </Badge>
        ) : null}
        {status && status.failed > 0 ? (
          <Badge variant="destructive">
            {t('row.failed', { count: status.failed })}
          </Badge>
        ) : null}
        <Badge variant={sink.identity_enabled ? 'warning' : 'muted'}>
          {sink.identity_enabled ? t('row.identity') : t('row.noIdentity')}
        </Badge>
      </div>
      <div className="flex items-center gap-2">
        <Switch
          size="sm"
          checked={sink.enabled}
          disabled={update.isPending}
          aria-label={t('row.enabled')}
          onCheckedChange={(checked) => {
            update.mutate({ id: sink.id, body: { enabled: checked } })
          }}
        />
        <Button
          variant="outline"
          size="sm"
          disabled={test.isPending}
          onClick={runTest}
        >
          <PaperPlaneTiltIcon className="size-3.5" aria-hidden="true" />
          {test.isPending ? t('row.testing') : t('row.test')}
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label={t('row.edit')}
          onClick={() => {
            onEdit(sink)
          }}
        >
          <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label={t('row.remove')}
          onClick={() => {
            setConfirmOpen(true)
          }}
        >
          <TrashIcon className="size-3.5" aria-hidden="true" />
        </Button>
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('remove.title', { name: sink.name })}
        description={t('remove.description')}
        confirmLabel={t('remove.confirm')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() => {
          remove.mutate(sink.id, {
            onSuccess: () => {
              setConfirmOpen(false)
            },
          })
        }}
      />
    </li>
  )
}
