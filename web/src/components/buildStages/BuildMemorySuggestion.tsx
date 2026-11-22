import { useTranslation } from 'react-i18next'
import { GaugeGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  useBuildStats,
  useSetAppBuildLimits,
} from '../../queries/buildPerformance'

/** After an out of memory build: one click sets the app's build memory from the observed peak, or points to a build node. */
export function BuildMemorySuggestion({ appName }: { appName: string }) {
  const { t } = useTranslation('buildMemory')
  const isRoot = useIsRoot()
  const stats = useBuildStats(appName, 1)
  const save = useSetAppBuildLimits(appName)
  const s = stats.data?.memory_suggestion
  if (!s) return null
  return (
    <div
      data-testid="build-memory-suggestion"
      className="flex flex-wrap items-center gap-3 rounded-md border p-3"
    >
      <GaugeGlassIcon aria-hidden="true" className="size-5" />
      <div className="min-w-0 flex-1 space-y-1 text-sm">
        <p className="font-medium">{t('title')}</p>
        <p className="text-muted-foreground">
          {s.peak_mb > 0
            ? t('peak', { peak: s.peak_mb, limit: s.limit_mb })
            : t('capOnly', { limit: s.limit_mb })}
        </p>
        {s.route_to_node ? (
          <p className="text-muted-foreground">
            {t('routeHint', { value: s.suggested_mb })}
          </p>
        ) : (
          <p className="text-muted-foreground">{t('setCapHint')}</p>
        )}
        {!isRoot ? (
          <p className="text-muted-foreground">{t('rootOnly')}</p>
        ) : null}
      </div>
      {s.route_to_node ? (
        <Button size="sm" render={<a href="/nodes" />}>
          {t('routeToNode')}
        </Button>
      ) : (
        <Button
          type="button"
          size="sm"
          disabled={!isRoot || save.isPending}
          onClick={() => {
            save.mutate(
              { memory_mb: s.suggested_mb },
              {
                onSuccess: () => {
                  toast.add({
                    title: t('saved', { value: s.suggested_mb }),
                    type: 'success',
                  })
                },
              },
            )
          }}
        >
          {t('setCap', { value: s.suggested_mb })}
        </Button>
      )}
    </div>
  )
}
