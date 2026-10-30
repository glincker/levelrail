import { useMemo, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { CodeGlassIcon } from '@thesvg/ui-react'
import { openAPISpecQueryOptions, useOpenAPISpec } from '@/queries/openapi'
import type { OpenAPIRoute } from '@/queries/openapi'
import { ApiExplorerRoute } from '@/components/settings/ApiExplorerRoute'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { ListSkeleton } from '@/components/ui/list-skeleton'

export const Route = createFileRoute('/settings/api-explorer')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(openAPISpecQueryOptions()),
  component: ApiExplorerPage,
  pendingComponent: ApiExplorerPending,
})

export function groupRoutes(
  routes: OpenAPIRoute[],
): Map<string, OpenAPIRoute[]> {
  const groups = new Map<string, OpenAPIRoute[]>()
  for (const route of routes) {
    const key = route.group || 'Other'
    const existing = groups.get(key)
    if (existing) {
      existing.push(route)
    } else {
      groups.set(key, [route])
    }
  }
  return groups
}

export function matchesFilter(route: OpenAPIRoute, filter: string): boolean {
  if (!filter) return true
  const haystack =
    `${route.method} ${route.path} ${route.ability} ${route.description ?? ''}`.toLowerCase()
  return haystack.includes(filter.toLowerCase())
}

function ApiExplorerPage() {
  const { t } = useTranslation('settings')
  const { data } = useOpenAPISpec()
  const [filter, setFilter] = useState('')

  const filteredGroups = useMemo(() => {
    const filtered = data.routes.filter((r) => matchesFilter(r, filter))
    return groupRoutes(filtered)
  }, [data.routes, filter])

  const coveragePct = Math.round(
    (data.exampleCount / Math.max(data.count, 1)) * 100,
  )

  return (
    <PageTemplate
      icon={<CodeGlassIcon size="1.4em" />}
      title={t('apiExplorerPage.title')}
      description={t('apiExplorerPage.description')}
    >
      <div className="space-y-4">
        <Input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder={t('apiExplorerPage.filterPlaceholder')}
          aria-label={t('apiExplorerPage.filterLabel')}
        />
        <p className="text-sm text-muted-foreground">
          {t('apiExplorerPage.summary', {
            count: data.count,
            examples: data.exampleCount,
            percent: coveragePct,
          })}
        </p>
      </div>

      <div className="space-y-6">
        {[...filteredGroups.entries()].map(([group, routes]) => (
          <Card key={group}>
            <CardHeader>
              <CardTitle className="flex items-center justify-between">
                <span>{group}</span>
                <span className="text-xs font-normal text-muted-foreground">
                  {t('apiExplorerPage.routeCount', { count: routes.length })}
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="divide-y divide-border">
              {routes.map((route) => (
                <ApiExplorerRoute
                  key={`${route.method} ${route.path}`}
                  route={route}
                />
              ))}
            </CardContent>
          </Card>
        ))}
        {filteredGroups.size === 0 && (
          <p className="text-sm text-muted-foreground">
            {t('apiExplorerPage.noMatch', { filter })}
          </p>
        )}
      </div>
    </PageTemplate>
  )
}

function ApiExplorerPending() {
  const { t } = useTranslation('settings')
  return (
    <PageTemplate
      title={t('apiExplorerPage.title')}
      icon={<CodeGlassIcon size="1.4em" />}
    >
      <ListSkeleton rows={8} />
    </PageTemplate>
  )
}
