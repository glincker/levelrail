import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

/** GoLiveCta is the primary next step once an app runs: put it on a domain with https. */
export function GoLiveCta({ app }: { app: string }) {
  const { t } = useTranslation('golive')
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-tone-accent-border bg-tone-accent-soft p-4">
      <div className="min-w-0 space-y-1">
        <p className="flex items-center gap-2 text-sm font-semibold">
          <RocketGlassIcon className="size-5" aria-hidden="true" />
          {t('cta.title')}
        </p>
        <p className="text-xs text-muted-foreground">{t('cta.body')}</p>
      </div>
      <Button
        size="sm"
        render={<Link to="/apps/$name/go-live" params={{ name: app }} />}
        nativeButton={false}
      >
        {t('cta.button')}
      </Button>
    </div>
  )
}
