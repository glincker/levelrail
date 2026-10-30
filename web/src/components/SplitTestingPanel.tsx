import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FlaskConicalGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { InfoTip } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import {
  useSplitActions,
  useSplitTest,
  type SplitTest,
  type SplitVariant,
} from '../queries/splitTesting'
import { SplitStartForm } from './SplitStartForm'
import { SplitVariantsTable } from './SplitVariantsTable'

const HOURLY_ROWS = 12
const SECONDS_PER_HOUR = 3600

function HourlyStats({ test }: { test: SplitTest }) {
  const { t } = useTranslation('splitTesting')
  const rows = (test.hourly ?? []).slice(-HOURLY_ROWS)
  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-foreground">
        {t('stats.title')}
      </h3>
      <p className="text-xs text-muted-foreground">
        {t('stats.total', { count: test.total_requests })}
      </p>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('stats.none')}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-border text-left text-muted-foreground">
                <th scope="col" className="py-1 pr-3 font-medium">
                  {t('stats.hour')}
                </th>
                {test.variants.map((v) => (
                  <th
                    key={v.id}
                    scope="col"
                    className="py-1 pr-3 text-right font-medium"
                  >
                    {v.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.bucket} className="border-b border-border/60">
                  <td className="py-1 pr-3 text-muted-foreground">
                    {new Date(r.bucket).toLocaleString()}
                  </td>
                  {test.variants.map((v) => (
                    <td
                      key={v.id}
                      className="py-1 pr-3 text-right tabular-nums"
                    >
                      {r.counts[v.id] ?? 0}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

/** SplitTestingPanel runs sticky percentage splits of an app's visitors between deploys. */
export function SplitTestingPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('splitTesting')
  const { data, isPending, error } = useSplitTest(appName)
  const { setWeights, promote, stop } = useSplitActions(appName)
  const [promoting, setPromoting] = useState<SplitVariant | null>(null)
  const [stopping, setStopping] = useState(false)
  const busy = setWeights.isPending || promote.isPending || stop.isPending

  function fail(err: Error) {
    toast.add({ title: t('error'), description: err.message, type: 'error' })
  }

  let body
  if (isPending) {
    body = (
      <p className="text-sm text-muted-foreground" role="status">
        {t('loading')}
      </p>
    )
  } else if (error || !data) {
    body = (
      <p className="text-sm text-destructive" role="alert">
        {t('loadError', { message: error?.message ?? '' })}
      </p>
    )
  } else if (!data.supported) {
    body = (
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">
          {t('unsupportedTitle')}
        </p>
        <p className="text-sm text-muted-foreground">{t('unsupportedHelp')}</p>
      </div>
    )
  } else if (data.active) {
    body = (
      <div className="space-y-5">
        <SplitVariantsTable
          test={data}
          busy={busy}
          onSaveWeights={(weights) =>
            setWeights.mutate(weights, {
              onSuccess: () =>
                toast.add({ title: t('table.weightsSaved'), type: 'success' }),
              onError: fail,
            })
          }
          onPromote={setPromoting}
        />
        <p className="text-xs text-muted-foreground">
          {t('cookie.summary', {
            name: data.cookie_name,
            hours: Math.round((data.cookie_ttl_secs ?? 0) / SECONDS_PER_HOUR),
            param: data.override_param,
          })}
        </p>
        <HourlyStats test={data} />
        <div className="border-t border-border pt-3">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => setStopping(true)}
          >
            {t('stop.button')}
          </Button>
        </div>
      </div>
    )
  } else {
    body = (
      <div className="space-y-5">
        {data.test_id ? (
          <div className="space-y-2">
            <h3 className="text-sm font-medium text-foreground">
              {t('ended.title')}
            </h3>
            <p className="text-xs text-muted-foreground">
              {t('ended.reason', { reason: data.end_reason })}
            </p>
            <HourlyStats test={data} />
          </div>
        ) : (
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">
              {t('empty.title')}
            </p>
            <p className="text-sm text-muted-foreground">{t('empty.body')}</p>
          </div>
        )}
        <SplitStartForm appName={appName} />
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FlaskConicalGlassIcon className="size-5 text-muted-foreground" />
          {t('title')}
          <InfoTip label={t('title')}>{t('description')}</InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
      <ConfirmDialog
        open={stopping}
        onOpenChange={setStopping}
        title={t('stop.title')}
        description={t('stop.body')}
        confirmLabel={t('stop.confirm')}
        pending={stop.isPending}
        onConfirm={() =>
          stop.mutate(undefined, {
            onSuccess: () => {
              setStopping(false)
              toast.add({ title: t('stop.success'), type: 'success' })
            },
            onError: fail,
          })
        }
      />
      <ConfirmDialog
        open={promoting !== null}
        onOpenChange={(open) => {
          if (!open) setPromoting(null)
        }}
        title={t('promote.title', { label: promoting?.label })}
        description={t('promote.body', { label: promoting?.label })}
        consequences={[t('promote.locked')]}
        confirmLabel={t('promote.confirm')}
        pending={promote.isPending}
        onConfirm={() => {
          if (!promoting) return
          promote.mutate(promoting.id, {
            onSuccess: () => {
              setPromoting(null)
              toast.add({ title: t('promote.success'), type: 'success' })
            },
            onError: (err) => {
              setPromoting(null)
              fail(err)
            },
          })
        }}
      />
    </Card>
  )
}
