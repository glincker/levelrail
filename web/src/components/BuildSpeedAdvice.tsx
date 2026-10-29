import { Link } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ZapGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { formatBuildSeconds } from '../lib/buildDuration'
import {
  buildPerformanceKeys,
  useBuildSpeed,
  type BuildSpeedFinding,
} from '../queries/buildPerformance'
import { useSetBuildCache } from '../queries/buildCache'

function FindingRow({
  appName,
  finding,
}: {
  appName: string
  finding: BuildSpeedFinding
}) {
  const { t } = useTranslation('buildPerformance')
  const qc = useQueryClient()
  const setCache = useSetBuildCache()

  function applyAuto() {
    if (!finding.target_id) return
    setCache.mutate(
      {
        app_name: appName,
        target_id: finding.target_id,
        mode: 'max',
        enabled: true,
      },
      {
        onSuccess: () => {
          toast.add({ title: t('speed.fixed'), type: 'success' })
          void qc.invalidateQueries({
            queryKey: buildPerformanceKeys.speed(appName),
          })
        },
        onError: (error) =>
          toast.add({
            title: t('speed.fixError'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <li className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0 space-y-1">
        <p className="text-sm font-medium">
          {t(`speed.findings.${finding.code}.title`)}
        </p>
        <p className="text-sm text-muted-foreground">
          {t(`speed.findings.${finding.code}.body`)}
        </p>
      </div>
      {finding.fix_kind === 'auto' ? (
        <Button size="sm" disabled={setCache.isPending} onClick={applyAuto}>
          {setCache.isPending ? t('speed.fixing') : t('speed.fix.auto')}
        </Button>
      ) : null}
      {finding.fix_kind === 'guided' ? (
        <Button
          size="sm"
          variant="outline"
          nativeButton={false}
          render={
            <Link to="/apps/$name/deploy-settings" params={{ name: appName }} />
          }
        >
          {t('speed.fix.guided')}
        </Button>
      ) : null}
    </li>
  )
}

export function BuildSpeedAdvice({ appName }: { appName: string }) {
  const { t } = useTranslation('buildPerformance')
  const { data, error } = useBuildSpeed(appName)

  if (error) {
    return (
      <p role="alert" className="text-sm text-muted-foreground">
        {t('speed.loadError')}
      </p>
    )
  }
  if (!data) return null

  return (
    <section className="space-y-2 border-t pt-4">
      <h3 className="flex items-center gap-2 text-sm font-semibold">
        <ZapGlassIcon aria-hidden="true" className="size-4.5" />
        {t('speed.title')}
      </h3>
      {data.warm_known && data.time_saved_seconds > 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('speed.saved', {
            saved: formatBuildSeconds(data.time_saved_seconds),
            count: data.builds,
            cold: formatBuildSeconds(data.cold_p50_seconds),
            warm: formatBuildSeconds(data.warm_p50_seconds),
          })}
        </p>
      ) : null}
      {data.findings.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('speed.none')}</p>
      ) : (
        <ul className="divide-y">
          {data.findings.map((f) => (
            <FindingRow key={f.code} appName={appName} finding={f} />
          ))}
        </ul>
      )}
    </section>
  )
}
