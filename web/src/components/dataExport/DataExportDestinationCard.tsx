import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CirclePauseGlassIcon,
  SendGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useDeleteDataExportDestination,
  useTestDataExportDestination,
  useUpdateDataExportDestination,
} from '../../queries/dataExport'
import type {
  DataExportDestination,
  DataExportKind,
  DataExportTestResult,
  UpdateDataExportDestinationRequest,
} from '../../types/dataExport'
import { TestFixPanel } from './TestFixPanel'

const KIND_KEYS = {
  logs_http: 'kinds.logs_http',
  logs_loki: 'kinds.logs_loki',
  logs_otlp: 'kinds.logs_otlp',
  logs_syslog: 'kinds.logs_syslog',
  logs_newrelic: 'kinds.logs_newrelic',
  logs_datadog: 'kinds.logs_datadog',
  logs_axiom: 'kinds.logs_axiom',
  logs_betterstack: 'kinds.logs_betterstack',
  audit_webhook: 'kinds.audit_webhook',
  audit_s3: 'kinds.audit_s3',
  audit_splunk: 'kinds.audit_splunk',
  audit_syslog: 'kinds.audit_syslog',
  audit_otlp: 'kinds.audit_otlp',
  metrics_otlp: 'kinds.metrics_otlp',
  metrics_remote_write: 'kinds.metrics_remote_write',
} as const satisfies Record<DataExportKind, string>

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm text-foreground">{value}</dd>
    </div>
  )
}

export function DataExportDestinationCard({
  destination,
}: {
  destination: DataExportDestination
}) {
  const { t } = useTranslation('dataExport')
  const update = useUpdateDataExportDestination()
  const test = useTestDataExportDestination()
  const remove = useDeleteDataExportDestination()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [failure, setFailure] = useState<DataExportTestResult | null>(null)
  const s = destination.status

  const endpoint =
    destination.kind === 'audit_s3'
      ? (destination.options.storage_target_id ?? '')
      : destination.endpoint

  function applyFix(body: UpdateDataExportDestinationRequest) {
    update.mutate(
      { id: destination.id, body },
      {
        onSuccess: () => {
          runTest()
        },
      },
    )
  }

  function runTest() {
    test.mutate(destination.id, {
      onSuccess: (res) => {
        setFailure(res.ok ? null : res)
        if (res.ok) {
          toast.add({
            title: t('card.testOk', { name: destination.name }),
            type: 'success',
          })
          return
        }
        toast.add({
          title: t('card.testFailed', {
            name: destination.name,
            message: res.error ?? '',
          }),
          type: 'error',
        })
      },
      onError: (err) => {
        toast.add({
          title: t('card.testFailed', {
            name: destination.name,
            message: err.message,
          }),
          type: 'error',
        })
      },
    })
  }

  let health = (
    <Badge variant="success">
      <CircleCheckGlassIcon
        className="size-3.5 text-tone-success"
        aria-hidden="true"
      />
      {t('card.circuitClosed')}
    </Badge>
  )
  if (!destination.enabled) {
    health = (
      <Badge variant="muted">
        <CirclePauseGlassIcon className="size-3.5" aria-hidden="true" />
        {t('card.paused')}
      </Badge>
    )
  } else if (s.circuit_open || s.last_error) {
    health = (
      <Badge variant="warning">
        <CircleAlertGlassIcon
          className="size-3.5 text-tone-danger"
          aria-hidden="true"
        />
        {s.circuit_open ? t('card.circuitOpen') : t('card.lastError')}
      </Badge>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          {destination.name}
          {health}
        </CardTitle>
        <CardDescription className="break-all">
          {t(KIND_KEYS[destination.kind])}
          {endpoint ? ` · ${endpoint}` : ''}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat
            label={t('card.lastSuccess')}
            value={s.last_success_at || t('card.never')}
          />
          <Stat label={t('card.queue')} value={String(s.queue_depth)} />
          <Stat label={t('card.sent')} value={String(s.sent_total)} />
          <Stat label={t('card.dropped')} value={String(s.dropped_total)} />
        </dl>
        {s.last_error ? (
          <p className="break-words text-sm text-tone-danger">
            {t('card.lastError')}: {s.last_error}
          </p>
        ) : null}
        {failure?.fix ? (
          <TestFixPanel
            destination={destination}
            fix={failure.fix}
            message={failure.error ?? ''}
            saving={update.isPending || test.isPending}
            onApply={applyFix}
            onRetest={runTest}
          />
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <label className="flex items-center gap-2 text-sm text-foreground">
            <Switch
              checked={destination.enabled}
              disabled={update.isPending}
              onCheckedChange={(checked) => {
                update.mutate({
                  id: destination.id,
                  body: { enabled: checked },
                })
              }}
            />
            {t('card.enabled')}
          </label>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={test.isPending}
              onClick={runTest}
            >
              <SendGlassIcon className="size-4" aria-hidden="true" />
              {test.isPending ? t('card.testing') : t('card.test')}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                setConfirmOpen(true)
              }}
            >
              <TrashGlassIcon className="size-4" aria-hidden="true" />
              {t('card.delete')}
            </Button>
          </div>
        </div>
      </CardContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        tone="destructive"
        title={t('card.deleteTitle', { name: destination.name })}
        description={t('card.deleteDescription')}
        confirmLabel={t('card.deleteConfirm')}
        pendingLabel={t('card.deleting')}
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : null}
        onConfirm={() => {
          remove.mutate(destination.id, {
            onSuccess: () => {
              setConfirmOpen(false)
            },
          })
        }}
      />
    </Card>
  )
}
