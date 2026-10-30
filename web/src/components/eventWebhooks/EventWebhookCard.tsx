import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  KeyGlassIcon,
  SendGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
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
  useDeleteEventWebhook,
  useRotateEventWebhookSecret,
  useTestEventWebhook,
  useUpdateEventWebhook,
} from '../../queries/eventWebhooks'
import type { EventWebhook } from '../../types/eventWebhooks'
import { EventWebhookDeliveries } from './EventWebhookDeliveries'

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm text-foreground">{value}</dd>
    </div>
  )
}

export function EventWebhookCard({ webhook }: { webhook: EventWebhook }) {
  const { t } = useTranslation('eventWebhooks')
  const update = useUpdateEventWebhook()
  const test = useTestEventWebhook()
  const rotate = useRotateEventWebhookSecret()
  const remove = useDeleteEventWebhook()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [showLog, setShowLog] = useState(false)
  const [newSecret, setNewSecret] = useState<string | null>(null)

  function runTest() {
    test.mutate(webhook.id, {
      onSuccess: (res) => {
        toast.add({
          title: res.ok
            ? t('card.testOk', { name: webhook.name })
            : t('card.testFailed', {
                name: webhook.name,
                message: res.error ?? '',
              }),
          type: res.ok ? 'success' : 'error',
        })
      },
      onError: (err) => {
        toast.add({
          title: t('card.testFailed', {
            name: webhook.name,
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
          {webhook.name}
          {webhook.dead > 0 ? (
            <Badge variant="destructive">
              {t('card.deadBadge', { count: webhook.dead })}
            </Badge>
          ) : null}
          {!webhook.enabled ? (
            <Badge variant="muted">{t('card.paused')}</Badge>
          ) : null}
        </CardTitle>
        <CardDescription className="break-all">
          {webhook.url}
          {' · '}
          {webhook.events.length === 0
            ? t('card.allEvents')
            : webhook.events.join(', ')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label={t('card.delivered')} value={String(webhook.delivered)} />
          <Stat label={t('card.pending')} value={String(webhook.pending)} />
          <Stat label={t('card.dead')} value={String(webhook.dead)} />
          <Stat
            label={t('card.lastStatus')}
            value={webhook.last_status ? String(webhook.last_status) : '-'}
          />
        </dl>
        {newSecret ? (
          <Alert>
            <AlertDescription className="space-y-1">
              <span className="block">{t('card.secretOnce')}</span>
              <code className="block break-all font-mono text-xs">
                {newSecret}
              </code>
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <label className="flex items-center gap-2 text-sm text-foreground">
            <Switch
              checked={webhook.enabled}
              disabled={update.isPending}
              onCheckedChange={(checked) => {
                update.mutate({ id: webhook.id, body: { enabled: checked } })
              }}
            />
            {t('card.enabled')}
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
              <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
              {t('card.log')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={test.isPending}
              onClick={runTest}
            >
              <SendGlassIcon className="size-5" aria-hidden="true" />
              {test.isPending ? t('card.testing') : t('card.test')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={rotate.isPending}
              onClick={() => {
                rotate.mutate(webhook.id, {
                  onSuccess: (res) => {
                    setNewSecret(res.signing_secret ?? null)
                  },
                })
              }}
            >
              <KeyGlassIcon className="size-5" aria-hidden="true" />
              {t('card.rotate')}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                setConfirmOpen(true)
              }}
            >
              <TrashGlassIcon className="size-5" aria-hidden="true" />
              {t('card.delete')}
            </Button>
          </div>
        </div>
        {showLog ? <EventWebhookDeliveries webhookId={webhook.id} /> : null}
      </CardContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        tone="destructive"
        title={t('card.deleteTitle', { name: webhook.name })}
        description={t('card.deleteDescription')}
        confirmLabel={t('card.deleteConfirm')}
        pendingLabel={t('card.deleting')}
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : null}
        onConfirm={() => {
          remove.mutate(webhook.id, {
            onSuccess: () => {
              setConfirmOpen(false)
            },
          })
        }}
      />
    </Card>
  )
}
