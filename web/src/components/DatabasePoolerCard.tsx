import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  databasePoolerQuery,
  useSetDatabasePooler,
} from '../queries/databasePooler'
import type { DatabaseResource } from '../types/databaseDetail'

function CopyRow({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('databases')
  const [copied, setCopied] = useState(false)
  function copy() {
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <div className="flex items-center gap-2">
      <span className="w-16 shrink-0 text-xs text-muted-foreground">
        {label}
      </span>
      <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1 font-mono text-xs">
        {value}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={copy}
        aria-label={t('pooler.copy', { label })}
      >
        {copied ? (
          <CheckGlassIcon size="1.2em" aria-hidden />
        ) : (
          <CopyGlassIcon size="1.2em" aria-hidden />
        )}
      </Button>
    </div>
  )
}

export function DatabasePoolerCard({
  database,
}: {
  database: DatabaseResource
}) {
  const { t } = useTranslation('databases')
  const { data } = useQuery(databasePoolerQuery(database.name))
  const setPooler = useSetDatabasePooler(database.name)
  const [poolSize, setPoolSize] = useState<string | null>(null)
  const [maxConn, setMaxConn] = useState<string | null>(null)

  if (!data?.supported) return null

  const blocked = !data.enabled && !data.image_configured

  function toggle(next: boolean) {
    setPooler.mutate(
      { enabled: next, ...tuning() },
      {
        onError: (error) =>
          toast.add({
            title: t('pooler.toggleFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function tuning() {
    const size = Number(poolSize ?? data?.pool_size ?? 0)
    const max = Number(maxConn ?? data?.max_client_conn ?? 0)
    return {
      poolSize: Number.isInteger(size) && size > 0 ? size : 0,
      maxClientConn: Number.isInteger(max) && max > 0 ? max : 0,
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>{t('pooler.title')}</CardTitle>
        <div className="flex items-center gap-2">
          {data.enabled && data.status ? (
            <span className="text-xs text-muted-foreground">
              {t(`pooler.status.${data.status}`)}
            </span>
          ) : null}
          <Switch
            checked={data.enabled}
            disabled={setPooler.isPending || blocked}
            onCheckedChange={toggle}
            aria-label={t('pooler.toggle')}
          />
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('pooler.description')}
        </p>
        {blocked ? (
          <p
            role="status"
            className="text-sm text-amber-600 dark:text-amber-400"
          >
            {t('pooler.imageMissing')}
          </p>
        ) : null}
        {data.enabled && data.status === 'failed' ? (
          <p role="alert" className="text-sm text-destructive">
            {data.message || data.reason || t('pooler.status.failed')}
          </p>
        ) : null}
        {data.pooled_url ? (
          <CopyRow label={t('pooler.pooled')} value={data.pooled_url} />
        ) : null}
        {data.direct_url ? (
          <CopyRow label={t('pooler.direct')} value={data.direct_url} />
        ) : null}
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1">
            <Label htmlFor="pooler-pool-size">{t('pooler.poolSize')}</Label>
            <Input
              id="pooler-pool-size"
              type="number"
              min={0}
              className="w-28"
              placeholder={t('pooler.default')}
              value={poolSize ?? (data.pool_size ? String(data.pool_size) : '')}
              onChange={(e) => setPoolSize(e.target.value)}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="pooler-max-conn">{t('pooler.maxClientConn')}</Label>
            <Input
              id="pooler-max-conn"
              type="number"
              min={0}
              className="w-28"
              placeholder={t('pooler.default')}
              value={
                maxConn ??
                (data.max_client_conn ? String(data.max_client_conn) : '')
              }
              onChange={(e) => setMaxConn(e.target.value)}
            />
          </div>
          {data.enabled ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={setPooler.isPending}
              onClick={() => toggle(true)}
            >
              {t('pooler.applySizing')}
            </Button>
          ) : null}
        </div>
        {data.enabled ? (
          <p className="text-xs text-muted-foreground">
            {t('pooler.remoteNote')}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
