import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { useBoundEnv } from '../../queries/envProtection'
import { Alert, AlertAction, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

interface BoundEnvSectionProps {
  app: string
}

/** BoundEnvSection lists the read-only variables injected from database links and bucket bindings. */
export function BoundEnvSection({ app }: BoundEnvSectionProps) {
  const { t } = useTranslation('envEditor')
  const bound = useBoundEnv(app)
  const rows = bound.data ?? []

  if (bound.isError) {
    return (
      <Alert>
        <AlertDescription>{t('bound.loadFailed')}</AlertDescription>
        <AlertAction>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void bound.refetch()}
          >
            {t('bound.retry')}
          </Button>
        </AlertAction>
      </Alert>
    )
  }
  if (rows.length === 0) return null

  return (
    <section className="space-y-2" aria-label={t('bound.title')}>
      <div>
        <h3 className="text-sm font-medium">{t('bound.title')}</h3>
        <p className="text-muted-foreground text-sm">
          {t('bound.description')}
        </p>
      </div>
      <ul className="divide-y rounded-md border">
        {rows.map((row) => (
          <li
            key={row.key}
            className="grid gap-1 px-3 py-2 sm:grid-cols-3 sm:gap-3"
          >
            <span className="font-mono text-sm break-all">{row.key}</span>
            <span className="text-muted-foreground text-sm">
              {row.source === 'database'
                ? t('bound.database', {
                    name: row.source_name,
                    field: row.field,
                  })
                : t('bound.bucket', { name: row.source_name })}
            </span>
            {row.masked ? (
              <Badge variant="outline" className="w-fit gap-1">
                <LockGlassIcon size="1.2em" aria-hidden />
                {t('bound.masked')}
              </Badge>
            ) : (
              <span className="font-mono text-sm break-all">{row.value}</span>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
