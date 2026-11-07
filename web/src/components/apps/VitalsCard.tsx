import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { SnippetBlock } from './VitalsSnippet'
import {
  formatVitalValue,
  useAppVitals,
  useAppVitalsActions,
  type VitalsMetric,
  type VitalsStat,
} from '../../queries/appVitals'

const VitalsRoutesDialog = lazy(() => import('./VitalsRoutesDialog'))

const HEADLINE: VitalsMetric[] = ['LCP', 'INP', 'CLS']

function ratingTone(stat?: VitalsStat): string {
  if (stat?.rating === 'good') return 'text-tone-success'
  if (stat?.rating === 'needs-improvement') return 'text-tone-warning'
  if (stat?.rating === 'poor') return 'text-tone-danger'
  return 'text-muted-foreground'
}

// Above the top histogram bound the value is only a lower bound.
function formatOverflow(metric: VitalsMetric, bound: number): string {
  return metric === 'CLS' ? `>${bound}` : `>${bound / 1000} s`
}

function StatValue({
  metric,
  stat,
}: {
  metric: VitalsMetric
  stat?: VitalsStat
}) {
  if (!stat || stat.count === 0) {
    return <span className="text-muted-foreground">-</span>
  }
  return (
    <span className={ratingTone(stat)}>
      {stat.overflow
        ? formatOverflow(metric, stat.p75)
        : formatVitalValue(metric, stat.p75)}
    </span>
  )
}

/** VitalsCard shows real user web vitals for an app and owns the collection switch and beacon key. */
export function VitalsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('webVitals')
  const { data, isPending, isError } = useAppVitals(appName)
  const { setEnabled, rotate } = useAppVitalsActions(appName)
  const [routesOpen, setRoutesOpen] = useState(false)

  const fail = (error: Error) =>
    toast.add({
      title: t('errorTitle'),
      description: error.message,
      type: 'error',
    })

  if (isPending) return <Skeleton className="h-24 w-full" />
  if (isError) return null

  const hasData = Object.keys(data.overall).length > 0
  return (
    <section
      aria-label={t('title')}
      className="mt-4 space-y-3 rounded-lg border p-3"
    >
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{t('title')}</h3>
        <label className="flex items-center gap-2 text-xs">
          {t('collect')}
          <Switch
            size="sm"
            checked={data.enabled && data.public_key !== ''}
            disabled={setEnabled.isPending}
            onCheckedChange={(on) => setEnabled.mutate(on, { onError: fail })}
          />
        </label>
      </div>
      {data.public_key ? (
        <SnippetBlock app={appName} publicKey={data.public_key} />
      ) : data.enabled ? (
        <Button
          type="button"
          size="sm"
          disabled={rotate.isPending}
          onClick={() => rotate.mutate(undefined, { onError: fail })}
        >
          {t('createKey')}
        </Button>
      ) : null}
      {data.public_key ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={rotate.isPending}
          onClick={() => rotate.mutate(undefined, { onError: fail })}
        >
          {t('rotateKey')}
        </Button>
      ) : null}
      {!hasData ? (
        <p className="text-sm text-muted-foreground">
          {data.enabled ? t('emptyEnabled') : t('emptyDisabled')}
        </p>
      ) : (
        <>
          <dl className="grid grid-cols-3 gap-3">
            {HEADLINE.map((m) => (
              <div key={m}>
                <dt className="text-xs text-muted-foreground">
                  {t('p75Label', { metric: m })}
                </dt>
                <dd className="text-lg font-semibold">
                  <StatValue metric={m} stat={data.overall[m]} />
                </dd>
              </div>
            ))}
          </dl>
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">
              {t('windowNote', { days: data.days })}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setRoutesOpen(true)}
            >
              {t('viewAllRoutes', { count: data.routes.length })}
            </Button>
          </div>
          {routesOpen ? (
            <Suspense fallback={null}>
              <VitalsRoutesDialog
                appName={appName}
                data={data}
                open
                onOpenChange={setRoutesOpen}
              />
            </Suspense>
          ) : null}
        </>
      )}
    </section>
  )
}
