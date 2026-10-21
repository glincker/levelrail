import { useTranslation } from 'react-i18next'
import { BellSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import { Switch } from '@/components/ui/switch'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { toast } from '@/components/ui/toast'
import { AlertKindGuidance } from './AlertKindGuidance'
import { isAlertKindCopyKind } from './alertKindCopy'
import {
  useDefaultAlerts,
  useResetDefaultAlerts,
  useSetDefaultAlertEnabled,
  type DefaultAlert,
} from '../queries/alertDefaults'

const SEVERITY_VARIANT: Record<string, 'info' | 'warning' | 'destructive'> = {
  info: 'info',
  warning: 'warning',
  critical: 'destructive',
}

export function AlertDefaultsPanel() {
  const { t } = useTranslation('inbox')
  const defaults = useDefaultAlerts()
  const set = useSetDefaultAlertEnabled()
  const reset = useResetDefaultAlerts()

  function resetAll() {
    reset.mutate(undefined, {
      onSuccess: () =>
        toast.add({ title: t('defaults.resetToast'), type: 'success' }),
      onError: () =>
        toast.add({ title: t('defaults.resetError'), type: 'error' }),
    })
  }

  function severityLabel(severity: string): string {
    switch (severity) {
      case 'info':
        return t('defaults.severity.info')
      case 'warning':
        return t('defaults.severity.warning')
      case 'critical':
        return t('defaults.severity.critical')
      default:
        return severity
    }
  }

  function toggle(alert: DefaultAlert, enabled: boolean) {
    set.mutate(
      { id: alert.id, enabled },
      {
        onSuccess: () => {
          toast.add({
            title: t(
              enabled ? 'defaults.enabledToast' : 'defaults.disabledToast',
              {
                name: alert.name,
              },
            ),
            type: 'success',
          })
        },
        onError: () => {
          toast.add({
            title: t('defaults.toggleError', { name: alert.name }),
            type: 'error',
          })
        },
      },
    )
  }

  if (defaults.isPending) {
    return <TableSkeleton columnCount={2} />
  }
  if (defaults.isError) {
    return <p className="text-sm text-destructive">{t('defaults.loadError')}</p>
  }
  const list = defaults.data
  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {t('defaults.description')}
        </p>
        <Button
          variant="outline"
          size="sm"
          disabled={reset.isPending}
          onClick={resetAll}
        >
          {t('defaults.reset')}
        </Button>
      </div>
      {list.length === 0 ? (
        <EmptyState
          icon={<BellSimpleIcon className="size-5" aria-hidden="true" />}
          title={t('defaults.emptyTitle')}
          description={t('defaults.emptyBody')}
        />
      ) : (
        <ul className="divide-y rounded-lg border">
          {list.map((a) => (
            <li
              key={a.id}
              className="flex items-center justify-between gap-3 p-3"
            >
              <div className="min-w-0 space-y-1">
                <p className="truncate text-sm font-medium">{a.name}</p>
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant={SEVERITY_VARIANT[a.severity] ?? 'outline'}>
                    {severityLabel(a.severity)}
                  </Badge>
                  {a.firing ? (
                    <Badge variant="destructive">{t('defaults.firing')}</Badge>
                  ) : null}
                </div>
                {isAlertKindCopyKind(a.kind) ? (
                  <AlertKindGuidance kind={a.kind} />
                ) : null}
              </div>
              <Switch
                aria-label={t('defaults.toggleLabel', { name: a.name })}
                checked={a.enabled}
                disabled={set.isPending}
                onCheckedChange={(v) => toggle(a, v)}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
