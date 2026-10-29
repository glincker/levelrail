import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  PlusGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { closestNames } from '@/lib/closestNames'

export type NotFoundResource =
  'app' | 'database' | 'domain' | 'template' | 'project' | 'generic'

function SuggestionLink({
  resource,
  name,
}: {
  resource: NotFoundResource
  name: string
}) {
  const cls = 'font-medium text-foreground underline underline-offset-2'
  switch (resource) {
    case 'app':
      return (
        <Link to="/apps/$name/overview" params={{ name }} className={cls}>
          {name}
        </Link>
      )
    case 'database':
      return (
        <Link to="/databases/$name" params={{ name }} className={cls}>
          {name}
        </Link>
      )
    default:
      return null
  }
}

// A missing resource with a way forward: what was asked for, why it is
// probably gone, and real actions. Renders inside the page, not full screen.
export function NotFoundState({
  resource,
  name,
  candidates = [],
}: {
  resource: NotFoundResource
  name?: string
  /** Existing names (already cached) to offer as "did you mean" links. */
  candidates?: readonly string[]
}) {
  const { t } = useTranslation('common')
  const suggestions = name ? closestNames(name, candidates) : []
  const listLabel = t(`notFound.resources.${resource}.list`)
  return (
    <div
      className="flex min-h-[40vh] items-center justify-center p-4"
      data-testid="not-found-state"
    >
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-center gap-2">
            <SearchGlassIcon
              className="size-5.5 text-muted-foreground"
              aria-hidden="true"
            />
            <CardTitle>{t(`notFound.resources.${resource}.title`)}</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {name ? (
            <p className="break-all font-mono text-sm text-foreground">
              {t('notFound.requested', { name })}
            </p>
          ) : null}
          <p className="text-sm text-muted-foreground">
            {t('notFound.causes')}
          </p>
          {suggestions.length > 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('notFound.didYouMean')}{' '}
              {suggestions.map((s, i) => (
                <span key={s}>
                  {i > 0 ? ', ' : ''}
                  <SuggestionLink resource={resource} name={s} />
                </span>
              ))}
              {'?'}
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <ListLink resource={resource} label={listLabel} />
            {resource === 'app' ? (
              <Link to="/apps/new">
                <Button type="button" variant="outline" size="sm">
                  <PlusGlassIcon size="1.2em" aria-hidden="true" />
                  {t('notFound.resources.app.create')}
                </Button>
              </Link>
            ) : null}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function ListLink({
  resource,
  label,
}: {
  resource: NotFoundResource
  label: string
}) {
  const body = (
    <Button type="button" variant="outline" size="sm">
      <ArrowLeftGlassIcon size="1.2em" aria-hidden="true" />
      {label}
    </Button>
  )
  switch (resource) {
    case 'app':
      return <Link to="/apps">{body}</Link>
    case 'database':
      return <Link to="/databases">{body}</Link>
    case 'domain':
      return <Link to="/domains">{body}</Link>
    case 'template':
      return <Link to="/templates">{body}</Link>
    case 'project':
      return <Link to="/projects">{body}</Link>
    default:
      return <Link to="/">{body}</Link>
  }
}
