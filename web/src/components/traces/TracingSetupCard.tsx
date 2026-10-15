import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useDisableTracing,
  useEnableTracing,
  useTracing,
} from '../../queries/traces'
import type { Tracing } from '../../types/traces'

function CopyBlock({ value }: { value: string }) {
  const { t } = useTranslation('traces')
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-start gap-2 rounded-md bg-muted px-3 py-2">
      <pre className="min-w-0 flex-1 overflow-x-auto font-mono text-xs text-foreground">
        {value}
      </pre>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('setup.copy')}
        onClick={() => {
          void navigator.clipboard
            .writeText(value)
            .then(() => {
              setCopied(true)
              window.setTimeout(() => {
                setCopied(false)
              }, 1500)
            })
            .catch(() => undefined)
        }}
      >
        {copied ? (
          <CheckIcon aria-hidden="true" />
        ) : (
          <CopyIcon aria-hidden="true" />
        )}
      </Button>
    </div>
  )
}

/** TracingSetupCard turns the app's ingest key on and off and shows it once. */
export function TracingSetupCard({ app }: { app: string }) {
  const { t } = useTranslation('traces')
  const status = useTracing(app)
  const enable = useEnableTracing(app)
  const disable = useDisableTracing(app)
  const [fresh, setFresh] = useState<Tracing | null>(null)
  const enabled = status.data?.enabled ?? false
  const busy = enable.isPending || disable.isPending

  function onEnable() {
    enable.mutate(undefined, {
      onSuccess: (res) => {
        setFresh(res)
        toast.add({ title: t('setup.enabledToast'), type: 'success' })
      },
      onError: (e) => {
        toast.add({
          title: t('setup.failed'),
          description: e.message,
          type: 'error',
        })
      },
    })
  }

  function onDisable() {
    disable.mutate(undefined, {
      onSuccess: () => {
        setFresh(null)
        toast.add({ title: t('setup.disabledToast'), type: 'success' })
      },
      onError: (e) => {
        toast.add({
          title: t('setup.failed'),
          description: e.message,
          type: 'error',
        })
      },
    })
  }

  const env = fresh?.env
    ? Object.entries(fresh.env)
        .map(([k, v]) => `${k}=${v}`)
        .join('\n')
    : ''

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('setup.title')}</CardTitle>
        <CardDescription>
          {enabled ? t('setup.on') : t('setup.off')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {fresh?.key ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              {t('setup.keyOnce')}
            </p>
            <CopyBlock value={env} />
            <p className="text-xs text-muted-foreground">
              {t('setup.protocolNote')}
            </p>
          </div>
        ) : enabled && status.data ? (
          <div className="space-y-1">
            <p className="text-xs font-medium text-muted-foreground">
              {t('setup.endpoint')}
            </p>
            <CopyBlock value={status.data.endpoint} />
          </div>
        ) : null}
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" disabled={busy} onClick={onEnable}>
            {enable.isPending
              ? t('setup.enabling')
              : enabled
                ? t('setup.rotate')
                : t('setup.enable')}
          </Button>
          {enabled ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={onDisable}
            >
              {t('setup.disable')}
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
