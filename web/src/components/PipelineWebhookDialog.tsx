import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CopyGlassIcon,
  RefreshCwGlassIcon,
  TrashGlassIcon,
  WebhookGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useDeletePipelineWebhook,
  usePipelineWebhook,
  useRotatePipelineWebhook,
  useSetPipelineWebhook,
} from '../queries/pipelineExtras'
import type { Pipeline, PipelineWebhook } from '../types/pipelines'
import { PipelineWebhookDeliveries } from './PipelineWebhookDeliveries'

interface Minted {
  url: string
  secret?: string
}

function CopyField({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('pipelines')
  return (
    <div className="space-y-1">
      <Label>{label}</Label>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 break-all rounded-md bg-muted p-2 text-xs">
          {value}
        </code>
        <Button
          size="icon-sm"
          variant="outline"
          aria-label={`${t('webhook.copy')}: ${label}`}
          onClick={() =>
            void navigator.clipboard.writeText(value).then(() => {
              toast.add({ title: t('webhook.copied'), type: 'success' })
            })
          }
        >
          <CopyGlassIcon size="1.4em" aria-hidden="true" />
        </Button>
      </div>
    </div>
  )
}

function mintedFrom(res: PipelineWebhook): Minted | null {
  if (!res.token || !res.path) {
    return null
  }
  return { url: `${window.location.origin}${res.path}`, secret: res.secret }
}

function WebhookBody({
  appName,
  pipeline,
}: {
  appName: string
  pipeline: Pipeline
}) {
  const { t } = useTranslation('pipelines')
  const {
    data: hook,
    isLoading,
    error,
  } = usePipelineWebhook(appName, pipeline.name)
  const set = useSetPipelineWebhook(appName, pipeline.name)
  const rotate = useRotatePipelineWebhook(appName, pipeline.name)
  const del = useDeletePipelineWebhook(appName, pipeline.name)
  const [minted, setMinted] = useState<Minted | null>(null)
  const [wantHmac, setWantHmac] = useState(false)
  const fail = (e: Error) => toast.add({ title: e.message, type: 'error' })
  const accept = (res: PipelineWebhook) => {
    const m = mintedFrom(res)
    if (m || res.secret) {
      setMinted((prev) => m ?? (prev ? { ...prev, secret: res.secret } : null))
    }
  }

  if (isLoading) {
    return <Skeleton className="h-24 w-full" />
  }
  if (error || !hook) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {t('webhook.updateFailed')}: {error?.message}
        </AlertDescription>
      </Alert>
    )
  }
  const hasTrigger = (pipeline.triggers ?? []).includes('webhook')
  return (
    <div className="space-y-4">
      {!hasTrigger ? (
        <Alert>
          <AlertDescription>{t('webhook.needsFile')}</AlertDescription>
        </Alert>
      ) : null}
      {minted ? (
        <div className="space-y-3 rounded-lg border border-amber-300 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/30">
          <p className="text-xs text-foreground">{t('webhook.shownOnce')}</p>
          <CopyField label={t('webhook.url')} value={minted.url} />
          {minted.secret ? (
            <CopyField label={t('webhook.secret')} value={minted.secret} />
          ) : null}
        </div>
      ) : null}
      {!hook.configured ? (
        <div className="space-y-3">
          <p className="text-sm text-muted-foreground">
            {t('webhook.notConfigured')}
          </p>
          <div className="flex items-start gap-2">
            <Switch
              id="webhook-hmac-new"
              checked={wantHmac}
              onCheckedChange={setWantHmac}
              aria-label={t('webhook.hmac')}
            />
            <div>
              <Label htmlFor="webhook-hmac-new">{t('webhook.hmac')}</Label>
              <p className="text-xs text-muted-foreground">
                {t('webhook.hmacHint')}
              </p>
            </div>
          </div>
          <Button
            disabled={set.isPending}
            onClick={() =>
              set.mutate(
                { hmac: wantHmac, enabled: true },
                { onSuccess: accept, onError: fail },
              )
            }
          >
            <WebhookGlassIcon size="1.4em" aria-hidden="true" />
            {t('webhook.enable')}
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="webhook-enabled">{t('webhook.enabled')}</Label>
            <Switch
              id="webhook-enabled"
              checked={hook.enabled ?? false}
              disabled={set.isPending}
              onCheckedChange={(enabled) =>
                set.mutate({ enabled }, { onError: fail })
              }
            />
          </div>
          <div className="flex items-start justify-between gap-3">
            <div>
              <Label htmlFor="webhook-hmac">{t('webhook.hmac')}</Label>
              <p className="text-xs text-muted-foreground">
                {t('webhook.hmacHint')}
              </p>
            </div>
            <Switch
              id="webhook-hmac"
              checked={hook.hmac ?? false}
              disabled={set.isPending}
              onCheckedChange={(hmac) =>
                set.mutate({ hmac }, { onSuccess: accept, onError: fail })
              }
            />
          </div>
          {!minted && hook.path ? (
            <CopyField
              label={t('webhook.url')}
              value={`${window.location.origin}${hook.path}`}
            />
          ) : null}
          <p className="text-xs text-muted-foreground">
            {t('webhook.lastUsed', {
              when: hook.last_used_at
                ? new Date(hook.last_used_at).toLocaleString()
                : t('webhook.never'),
            })}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={rotate.isPending}
              onClick={() =>
                rotate.mutate(undefined, { onSuccess: accept, onError: fail })
              }
            >
              <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
              {t('webhook.rotate')}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              disabled={del.isPending}
              onClick={() =>
                del.mutate(undefined, {
                  onSuccess: () => setMinted(null),
                  onError: fail,
                })
              }
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
              {t('webhook.delete')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('webhook.rotateHint')}
          </p>
          <PipelineWebhookDeliveries app={appName} name={pipeline.name} />
        </div>
      )}
    </div>
  )
}

export function PipelineWebhookDialog({
  appName,
  pipeline,
}: {
  appName: string
  pipeline: Pipeline
}) {
  const { t } = useTranslation('pipelines')
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <WebhookGlassIcon size="1.4em" aria-hidden="true" />
        {t('webhook.button')}
      </DialogTrigger>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t('webhook.title', { name: pipeline.name })}
          </DialogTitle>
          <DialogDescription>{t('webhook.description')}</DialogDescription>
        </DialogHeader>
        {open ? <WebhookBody appName={appName} pipeline={pipeline} /> : null}
      </DialogContent>
    </Dialog>
  )
}
