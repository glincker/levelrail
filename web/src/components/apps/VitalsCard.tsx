import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  formatVitalValue,
  useAppVitals,
  useAppVitalsActions,
  type VitalsMetric,
  type VitalsStat,
  VITALS_METRICS,
} from '../../queries/appVitals'

const HEADLINE: VitalsMetric[] = ['LCP', 'INP', 'CLS']

function ratingTone(stat?: VitalsStat): string {
  if (stat?.rating === 'good') return 'text-tone-success'
  if (stat?.rating === 'needs-improvement') return 'text-tone-warning'
  if (stat?.rating === 'poor') return 'text-tone-danger'
  return 'text-muted-foreground'
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
      {formatVitalValue(metric, stat.p75)}
    </span>
  )
}

function snippetFor(app: string, key: string): string {
  const src = `${window.location.origin}/api/v1/vitals/snippet.js`
  return `<script defer src="${src}" data-app="${app}" data-key="${key}"></script>`
}

function SnippetBlock({ app, publicKey }: { app: string; publicKey: string }) {
  const { t } = useTranslation('webVitals')
  const { copied, copy } = useCopyToClipboard()
  const snippet = snippetFor(app, publicKey)
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{t('snippetHint')}</p>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto rounded bg-muted px-2 py-1 font-mono text-xs whitespace-nowrap">
          {snippet}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          aria-label={t('copySnippet')}
          onClick={() => copy(snippet)}
        >
          {copied ? (
            <CheckGlassIcon size="1em" />
          ) : (
            <CopyGlassIcon size="1em" />
          )}
          {copied ? t('copied') : t('copySnippet')}
        </Button>
      </div>
    </div>
  )
}

/** VitalsCard shows real user web vitals for an app and owns the collection switch and beacon key. */
export function VitalsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('webVitals')
  const { data, isPending, isError } = useAppVitals(appName)
  const { setEnabled, rotate } = useAppVitalsActions(appName)

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
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('route')}</TableHead>
                {VITALS_METRICS.map((m) => (
                  <TableHead key={m}>{m}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.routes.map((r) => (
                <TableRow key={r.route}>
                  <TableCell className="font-mono text-xs">{r.route}</TableCell>
                  {VITALS_METRICS.map((m) => (
                    <TableCell key={m}>
                      <StatValue metric={m} stat={r.metrics[m]} />
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <p className="text-xs text-muted-foreground">
            {t('windowNote', { days: data.days })}
          </p>
        </>
      )}
    </section>
  )
}
