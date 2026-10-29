import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FolderGlassIcon,
  LinkGlassIcon,
  TagGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { PurgeScope } from '../../queries/cdnCache'

const OPTIONS: { scope: PurgeScope; icon: ReactNode }[] = [
  { scope: 'url', icon: <LinkGlassIcon size="1.2em" aria-hidden="true" /> },
  {
    scope: 'prefix',
    icon: <FolderGlassIcon size="1.2em" aria-hidden="true" />,
  },
  { scope: 'tag', icon: <TagGlassIcon size="1.2em" aria-hidden="true" /> },
  {
    scope: 'all',
    icon: (
      <TriangleAlertGlassIcon
        className="text-tone-warning"
        size="1.2em"
        aria-hidden="true"
      />
    ),
  },
]

/** PurgeScopePicker chooses what a purge removes, as keyboard reachable radio cards. */
export function PurgeScopePicker({
  value,
  onChange,
  disabled = false,
}: Readonly<{
  value: PurgeScope
  onChange: (scope: PurgeScope) => void
  disabled?: boolean
}>) {
  const { t } = useTranslation('cdn')
  const name = useId()
  return (
    <fieldset className="min-w-0" disabled={disabled}>
      <legend className="sr-only">{t('purge.scopeLegend')}</legend>
      <div className="grid grid-cols-[repeat(auto-fit,minmax(11rem,1fr))] gap-3">
        {OPTIONS.map(({ scope, icon }) => {
          const destructive = scope === 'all'
          return (
            <label
              key={scope}
              className={cn(
                'relative flex cursor-pointer gap-3 rounded-lg border border-border bg-card p-3 text-left transition-colors',
                'hover:bg-muted/40 has-focus-visible:ring-2 has-focus-visible:ring-ring/60',
                'has-checked:border-foreground/40 has-checked:bg-muted/60',
                destructive &&
                  'has-checked:border-tone-danger-border has-checked:bg-tone-danger-soft',
                'has-disabled:cursor-not-allowed has-disabled:opacity-60',
              )}
            >
              <input
                type="radio"
                name={name}
                value={scope}
                checked={value === scope}
                onChange={() => onChange(scope)}
                className="peer sr-only"
              />
              <span
                className={cn(
                  'mt-0.5 shrink-0 text-muted-foreground [&_svg]:size-4',
                  destructive && 'text-tone-danger',
                )}
              >
                {icon}
              </span>
              <span className="min-w-0 space-y-0.5">
                <span className="block text-sm font-medium">
                  {t(`purge.scope.${scope}.title`)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(`purge.scope.${scope}.help`)}
                </span>
              </span>
            </label>
          )
        })}
      </div>
    </fieldset>
  )
}
