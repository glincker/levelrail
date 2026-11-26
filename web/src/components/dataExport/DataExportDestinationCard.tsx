import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CirclePauseGlassIcon,
  EllipsisGlassIcon,
  ShareGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { BrandIcon } from '../BrandIcon'
import { KIND_MARKS } from '../../lib/dataExportKinds'
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
        className="size-4.5 text-tone-success"
        aria-hidden="true"
      />
      {t('card.circuitClosed')}
    </Badge>
  )
  if (!destination.enabled) {
    health = (
      <Badge variant="muted">
        <CirclePauseGlassIcon className="size-4.5" aria-hidden="true" />
        {t('card.paused')}
      </Badge>
    )
  } else if (s.circuit_open || s.last_error) {
    health = (
      <Badge variant="warning">
        <CircleAlertGlassIcon
          className="size-4.5 text-tone-danger"
          aria-hidden="true"
        />
        {s.circuit_open ? t('card.circuitOpen') : t('card.lastError')}
      </Badge>
    )
  }

  const mark = KIND_MARKS[destination.kind]
  const sent = s.last_success_at
    ? new Date(s.last_success_at).toLocaleString()
    : t('card.never')

  return (
    <li className="space-y-2 px-3 py-2.5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="flex min-w-0 flex-1 items-center gap-2">
          {mark ? (
            <BrandIcon name={mark} className="size-5 shrink-0" />
          ) : (
            <ShareGlassIcon className="size-5 shrink-0" aria-hidden="true" />
          )}
          <span className="min-w-0">
            <span className="flex flex-wrap items-center gap-2">
              <span className="truncate text-sm font-medium">
                {destination.name}
              </span>
              <Badge variant="muted">{t(KIND_KEYS[destination.kind])}</Badge>
              {health}
            </span>
            {endpoint ? (
              <span className="block truncate text-xs text-muted-foreground">
                {endpoint}
              </span>
            ) : null}
          </span>
        </span>
        <span className="text-xs text-muted-foreground">
          {t('card.lastSuccess')}: {sent}
        </span>
        <label className="flex items-center gap-2 text-xs">
          <Switch
            checked={destination.enabled}
            disabled={update.isPending}
            aria-label={t('card.enabled')}
            onCheckedChange={(checked) => {
              update.mutate({ id: destination.id, body: { enabled: checked } })
            }}
          />
          {t('card.enabled')}
        </label>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="sm"
                aria-label={t('card.actions', { name: destination.name })}
              />
            }
          >
            <EllipsisGlassIcon className="size-5" aria-hidden="true" />
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem disabled={test.isPending} onClick={runTest}>
              {test.isPending ? t('card.testing') : t('card.test')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => {
                setConfirmOpen(true)
              }}
            >
              {t('card.delete')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {s.last_error ? (
        <p className="text-xs break-words text-tone-danger">
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
    </li>
  )
}
