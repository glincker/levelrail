import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  KeyIcon,
  PaperPlaneTiltIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
  useAuditSinkDeliveries,
  useDeleteAuditSink,
  useRotateAuditSinkSecret,
  useTestAuditSink,
  useUpdateAuditSink,
} from '../../queries/auditStream'
import type { AuditSink } from '../../types/auditStream'
import { AuditSinkFormDialog } from './AuditSinkFormDialog'

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm text-foreground">{value}</dd>
    </div>
  )
}

function targetOf(sink: AuditSink): string {
  switch (sink.kind) {
    case 'syslog':
      return sink.config.address ?? ''
    case 'webhook':
      return sink.config.url ?? ''
    case 's3':
      return [sink.config.endpoint, sink.config.bucket, sink.config.prefix]
        .filter(Boolean)
        .join(' / ')
  }
}

function SinkHistory({ sinkId }: { sinkId: string }) {
  const { t } = useTranslation('auditStream')
  const { data, isPending } = useAuditSinkDeliveries(sinkId, true)
  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('sinks.history.loading')}
      </p>
    )
  }
  if (!data || data.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('sinks.history.empty')}
      </p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-md border border-border">
      {data.map((d) => (
        <li key={d.id} className="space-y-0.5 px-3 py-2">
          <p className="flex flex-wrap items-center gap-2 text-sm text-foreground">
            <span className="font-mono">
              {t('sinks.history.range')} {d.from_seq}-{d.to_seq}
            </span>
            <Badge
              variant={d.state === 'delivered' ? 'success' : 'destructive'}
            >
              {d.state === 'delivered'
                ? t('sinks.history.delivered')
                : t('sinks.history.failed')}
            </Badge>
          </p>
          <p className="break-words text-xs text-muted-foreground">
            {t('sinks.history.entries')}: {d.entries} ·{' '}
            {t('sinks.history.attempt')}: {d.attempt} · {d.created_at}
            {d.last_error ? ` · ${d.last_error}` : ''}
          </p>
        </li>
      ))}
    </ul>
  )
}

export function AuditSinkCard({ sink }: { sink: AuditSink }) {
  const { t } = useTranslation('auditStream')
  const update = useUpdateAuditSink()
  const test = useTestAuditSink()
  const rotate = useRotateAuditSinkSecret()
  const remove = useDeleteAuditSink()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [showLog, setShowLog] = useState(false)
  const [newSecret, setNewSecret] = useState<string | null>(null)

  function runTest() {
    test.mutate(sink.id, {
      onSuccess: (res) => {
        toast.add({
          title: res.ok
            ? t('sinks.card.testOk', { name: sink.name })
            : t('sinks.card.testFailed', {
                name: sink.name,
                message: res.error ?? '',
              }),
          type: res.ok ? 'success' : 'error',
        })
      },
      onError: (err) => {
        toast.add({
          title: t('sinks.card.testFailed', {
            name: sink.name,
            message: err.message,
          }),
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          {sink.name}
          <Badge variant="muted">{t(`sinks.kind.${sink.kind}`)}</Badge>
          {sink.consecutive_failures > 0 ? (
            <Badge variant="destructive">
              {t('sinks.card.failing', { count: sink.consecutive_failures })}
            </Badge>
          ) : null}
          {!sink.enabled ? (
            <Badge variant="muted">{t('sinks.card.paused')}</Badge>
          ) : null}
        </CardTitle>
        <CardDescription className="break-all">
          {targetOf(sink)}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <Stat
            label={t('sinks.card.cursor')}
            value={String(sink.cursor_seq)}
          />
          <Stat
            label={t('sinks.card.lastDelivered')}
            value={sink.last_delivered_at ?? t('sinks.card.never')}
          />
          {sink.next_attempt_at ? (
            <Stat
              label={t('sinks.card.retryAt')}
              value={sink.next_attempt_at}
            />
          ) : null}
        </dl>
        {sink.last_error ? (
          <Alert variant="destructive">
            <AlertDescription className="break-words">
              {t('sinks.card.lastError')}: {sink.last_error}
            </AlertDescription>
          </Alert>
        ) : null}
        {newSecret ? (
          <Alert>
            <AlertDescription className="space-y-1">
              <span className="block">{t('sinks.card.secretOnce')}</span>
              <code className="block break-all font-mono text-xs">
                {newSecret}
              </code>
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <label className="flex items-center gap-2 text-sm text-foreground">
            <Switch
              checked={sink.enabled}
              disabled={update.isPending}
              onCheckedChange={(checked) => {
                update.mutate({ id: sink.id, body: { enabled: checked } })
              }}
            />
            {t('sinks.card.enabled')}
          </label>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              aria-expanded={showLog}
              onClick={() => {
                setShowLog((v) => !v)
              }}
            >
              <CaretDownIcon className="size-3.5" aria-hidden="true" />
              {t('sinks.card.history')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={test.isPending}
              onClick={runTest}
            >
              <PaperPlaneTiltIcon className="size-3.5" aria-hidden="true" />
              {test.isPending ? t('sinks.card.testing') : t('sinks.card.test')}
            </Button>
            {sink.kind === 'webhook' ? (
              <Button
                variant="outline"
                size="sm"
                disabled={rotate.isPending}
                onClick={() => {
                  rotate.mutate(sink.id, {
                    onSuccess: (res) => {
                      setNewSecret(res.signing_secret ?? null)
                    },
                  })
                }}
              >
                <KeyIcon className="size-3.5" aria-hidden="true" />
                {t('sinks.card.rotate')}
              </Button>
            ) : null}
            <AuditSinkFormDialog sink={sink} />
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                setConfirmOpen(true)
              }}
            >
              <TrashIcon className="size-3.5" aria-hidden="true" />
              {t('sinks.card.delete')}
            </Button>
          </div>
        </div>
        {showLog ? <SinkHistory sinkId={sink.id} /> : null}
      </CardContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        tone="destructive"
        title={t('sinks.card.deleteTitle', { name: sink.name })}
        description={t('sinks.card.deleteDescription')}
        confirmLabel={t('sinks.card.deleteConfirm')}
        pendingLabel={t('sinks.card.deleting')}
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : null}
        onConfirm={() => {
          remove.mutate(sink.id, {
            onSuccess: () => {
              setConfirmOpen(false)
            },
          })
        }}
      />
    </Card>
  )
}
