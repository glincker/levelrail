import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'

export type RoutesView = 'rows' | 'raw'

/** Rows/raw toggle and the diff switch, kept in one compact line. */
export function RoutesViewBar({
  view,
  onView,
  diff,
  onDiff,
  canDiff,
}: Readonly<{
  view: RoutesView
  onView: (v: RoutesView) => void
  diff: boolean
  onDiff: (v: boolean) => void
  canDiff: boolean
}>) {
  const { t } = useTranslation('pagesSites')
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div role="group" aria-label={t('routes.view.label')} className="flex">
        {(['rows', 'raw'] as const).map((v) => (
          <Button
            key={v}
            type="button"
            size="sm"
            variant={view === v ? 'secondary' : 'outline'}
            aria-pressed={view === v}
            onClick={() => onView(v)}
          >
            {t(`routes.view.${v}`)}
          </Button>
        ))}
      </div>
      <label className="flex items-center gap-1.5 text-xs">
        <Switch
          checked={diff}
          disabled={!canDiff}
          aria-label={t('routes.diff.toggle')}
          onCheckedChange={onDiff}
        />
        {t('routes.diff.toggle')}
      </label>
    </div>
  )
}
