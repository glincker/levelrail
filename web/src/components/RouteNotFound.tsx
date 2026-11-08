import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeftGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { suggestPages } from '@/lib/routeSuggestions'

// Root not-found page: renders inside the shell so the sidebar and header stay usable.
export function RouteNotFound() {
  const { t } = useTranslation(['common', 'settings', 'paletteSettings'])
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const suggestions = suggestPages(pathname)
  return (
    <section
      aria-labelledby="route-not-found-title"
      className="mx-auto flex min-h-[40vh] w-full max-w-lg flex-col justify-center gap-4 py-8"
      data-testid="route-not-found"
    >
      <div className="flex items-center gap-2">
        <SearchGlassIcon
          className="size-6 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <h1
          id="route-not-found-title"
          className="text-xl font-semibold text-foreground"
        >
          {t('common:a11y.routeNotFound.title')}
        </h1>
      </div>
      <p className="break-all font-mono text-sm text-foreground">
        {t('common:notFound.requested', { name: pathname })}
      </p>
      <p className="text-sm text-muted-foreground">
        {t('common:a11y.routeNotFound.body')}
      </p>
      {suggestions.length > 0 ? (
        <nav aria-label={t('common:a11y.routeNotFound.suggestions')}>
          <p className="mb-2 text-sm text-muted-foreground">
            {t('common:a11y.routeNotFound.suggestions')}
          </p>
          <ul className="flex flex-col gap-1">
            {suggestions.map((s) => (
              <li key={s.id}>
                <Link
                  to={s.path}
                  className="rounded-sm text-sm font-medium text-foreground underline underline-offset-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  {t(s.titleKey as 'common:a11y.routeNotFound.title')}
                </Link>{' '}
                <span className="font-mono text-xs text-muted-foreground">
                  {s.path}
                </span>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}
      <div>
        <Link
          to="/"
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          <ArrowLeftGlassIcon size="1.4em" aria-hidden="true" />
          {t('common:a11y.routeNotFound.home')}
        </Link>
      </div>
    </section>
  )
}

export default RouteNotFound
