import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useDensity } from '@/lib/density'
import { cn } from '@/lib/utils'

const NAV = ['apps', 'deployments', 'servers'] as const

/** AppearancePreview renders real components on live tokens, so it follows every option as it is picked. */
export function AppearancePreview() {
  const { t } = useTranslation('settings')
  const density = useDensity()
  const inputId = useId()
  const row = density === 'compact' ? 'py-1.5' : 'py-3'

  return (
    <div
      role="group"
      aria-label={t('appearancePage.hub.preview.title')}
      className="grid min-w-0 overflow-hidden rounded-2xl border border-border bg-background sm:grid-cols-[9rem_1fr]"
    >
      <nav
        aria-label={t('appearancePage.hub.preview.nav')}
        className="flex gap-1 border-b border-border bg-card p-2 sm:flex-col sm:border-r sm:border-b-0"
      >
        {NAV.map((k, i) => (
          <span
            key={k}
            aria-current={i === 0 ? 'page' : undefined}
            className={cn(
              'truncate rounded-lg px-2 py-1 text-sm',
              i === 0
                ? 'bg-state-selected font-medium text-foreground'
                : 'text-muted-foreground',
            )}
          >
            {t(`appearancePage.hub.preview.${k}`)}
          </span>
        ))}
      </nav>
      <div className="flex min-w-0 flex-col gap-3 p-3">
        <div className="divide-y divide-border rounded-xl border border-border bg-card px-3">
          {['web', 'api'].map((name, i) => (
            <div
              key={name}
              className={cn('flex items-center justify-between gap-2', row)}
            >
              <span className="truncate text-sm font-medium">{name}</span>
              <Badge variant={i === 0 ? 'accent' : 'outline'}>
                {t(
                  i === 0
                    ? 'appearancePage.hub.preview.running'
                    : 'appearancePage.hub.preview.stopped',
                )}
              </Badge>
            </div>
          ))}
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-0 flex-1 basis-40">
            <label htmlFor={inputId} className="mb-1 block text-xs">
              {t('appearancePage.hub.preview.field')}
            </label>
            <Input
              id={inputId}
              defaultValue={t('appearancePage.hub.preview.fieldValue')}
            />
          </div>
          <Button type="button" size="sm">
            {t('appearancePage.hub.preview.primary')}
          </Button>
          <Button type="button" size="sm" variant="outline">
            {t('appearancePage.hub.preview.secondary')}
          </Button>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <p className="glass glass-regular rounded-xl p-3 text-sm">
            {t('appearancePage.hub.preview.toast')}
          </p>
          <div className="glass glass-thick rounded-xl p-3">
            <p className="text-sm font-medium">
              {t('appearancePage.hub.preview.dialogTitle')}
            </p>
            <p className="mb-2 text-xs text-muted-foreground">
              {t('appearancePage.hub.preview.dialogBody')}
            </p>
            <Button type="button" size="sm" variant="destructive">
              {t('appearancePage.hub.preview.danger')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
