import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon, WrenchGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '@/hooks/useIsRoot'
import {
  useApplyControlPlaneFix,
  type CPHealthGuard,
} from '../../queries/controlPlaneHealth'

export function ReconcileGuardCard({ guard }: { guard: CPHealthGuard }) {
  const { t } = useTranslation('settings')
  const isRoot = useIsRoot()
  const fix = useApplyControlPlaneFix()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />
          {t('controlPlaneHealth.guard.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('controlPlaneHealth.guard.summary', {
            deferred: guard.passes_deferred,
            recent: guard.recent_limited,
            open: guard.open_breakers,
          })}
        </p>
        {guard.breakers.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('controlPlaneHealth.guard.none')}
          </p>
        ) : (
          guard.breakers.map((b) => (
            <div
              key={b.name}
              className="flex flex-wrap items-start justify-between gap-2 text-sm"
            >
              <div className="min-w-0">
                <span className="font-medium">{b.name}</span>
                {b.last_error ? (
                  <p className="truncate text-xs text-muted-foreground">
                    {b.last_error}
                  </p>
                ) : null}
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground tabular-nums">
                <Badge variant={b.state === 'open' ? 'destructive' : 'warning'}>
                  {t(`controlPlaneHealth.guard.state.${b.state}`)}
                </Badge>
                {t('controlPlaneHealth.guard.counts', {
                  trips: b.trips,
                  panics: b.panics,
                  throttled: b.throttled,
                })}
              </div>
            </div>
          ))
        )}
        {isRoot && (guard.open_breakers > 0 || guard.recent_limited > 0) ? (
          <Button
            size="sm"
            disabled={fix.isPending}
            onClick={() =>
              fix.mutate(
                { fix: 'reset_reconcile_guard' },
                {
                  onSuccess: () =>
                    toast.add({
                      title: t('controlPlaneHealth.fixApplied'),
                      type: 'success',
                    }),
                  onError: (e) =>
                    toast.add({
                      title: t('controlPlaneHealth.fixFailed'),
                      description: e.message,
                      type: 'error',
                    }),
                },
              )
            }
          >
            <WrenchGlassIcon size="1.4em" />
            {t('controlPlaneHealth.fixes.reset_reconcile_guard')}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}
