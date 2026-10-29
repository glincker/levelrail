import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GitCompareGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { usePendingDeployDiff } from '../../queries/pendingDeployDiff'
import type { DeployCompareEnvChange } from '../../types/deployCompare'

const ENV_VARIANT: Record<
  DeployCompareEnvChange['status'],
  'success' | 'destructive' | 'warning'
> = {
  added: 'success',
  removed: 'destructive',
  changed: 'warning',
}

const DEBOUNCE_MS = 400

// Shows what the next deploy changes against the live one: config fields and
// env keys only, never env values.
export function PendingChangesCard({
  appName,
  image = '',
}: {
  appName: string
  image?: string
}) {
  const { t } = useTranslation('smartDeploy')
  const [settled, setSettled] = useState(image)
  useEffect(() => {
    const id = setTimeout(() => {
      setSettled(image.trim())
    }, DEBOUNCE_MS)
    return () => {
      clearTimeout(id)
    }
  }, [image])
  const { data, isError } = usePendingDeployDiff(appName, settled)

  if (isError) {
    return (
      <p className="text-xs text-muted-foreground" role="status">
        {t('pending.loadFailed')}
      </p>
    )
  }
  if (!data) return null

  const envChanges = data.env_changes ?? []
  const empty = data.changes.length === 0 && envChanges.length === 0

  return (
    <section
      aria-label={t('pending.title')}
      className="space-y-2 rounded-lg border bg-muted/30 p-3"
    >
      <h3 className="flex items-center gap-1.5 text-sm font-medium">
        <GitCompareGlassIcon className="size-4.5" aria-hidden="true" />
        {t('pending.title')}
      </h3>
      {empty ? (
        <p className="text-xs text-muted-foreground">{t('pending.none')}</p>
      ) : (
        <ul className="space-y-1 text-xs">
          {data.changes.map((c) => (
            <li key={c.field} className="flex flex-wrap gap-x-1.5">
              <span className="font-medium">
                {t(`pending.field.${c.field}`, { defaultValue: c.field })}
              </span>
              <span className="font-mono text-muted-foreground">
                {c.from || t('pending.empty')}
                {' > '}
                {c.to || t('pending.empty')}
              </span>
            </li>
          ))}
          {envChanges.map((e) => (
            <li key={`env-${e.key}`} className="flex items-center gap-1.5">
              <Badge variant={ENV_VARIANT[e.status]}>
                {t(`pending.env.${e.status}`)}
              </Badge>
              <span className="font-mono">{e.key}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
