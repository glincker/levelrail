import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
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
        {copied ? <CheckIcon aria-hidden /> : <CopyIcon aria-hidden />}
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

  if (!data?.supported) return null

  const blocked = !data.enabled && !data.image_configured

  function toggle(next: boolean) {
    setPooler.mutate(next, {
      onError: (error) =>
        toast.add({
          title: t('pooler.toggleFailed'),
          description: error.message,
          type: 'error',
        }),
    })
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
        {data.enabled ? (
          <p className="text-xs text-muted-foreground">
            {t('pooler.remoteNote')}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
