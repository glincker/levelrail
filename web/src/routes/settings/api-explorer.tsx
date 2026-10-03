import { useMemo, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { CodeIcon } from '@phosphor-icons/react/dist/ssr'
import { openAPISpecQueryOptions, useOpenAPISpec } from '@/queries/openapi'
import type { OpenAPIRoute } from '@/queries/openapi'
import { ApiExplorerRoute } from '@/components/settings/ApiExplorerRoute'
import { PageHeader } from '@/components/shell/PageHeader'
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
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <CodeIcon className="size-4" />
        </div>
        <PageHeader
          title="API explorer"
          description="Browse and try every registered HTTP route on this instance, using your current session. Requests fire for real, same as hitting the endpoint with curl."
        />
      </div>

      <Input
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        placeholder="Filter by path, method, ability, or description…"
        aria-label="Filter routes"
      />

      <p className="text-sm text-muted-foreground">
        {data.count} routes, {data.exampleCount} with a worked example (
        {coveragePct}%). A route with no description just means its registration
        has no doc comment above it yet, not that it's unsupported.
      </p>

      <div className="space-y-6">
        {[...filteredGroups.entries()].map(([group, routes]) => (
          <Card key={group}>
            <CardHeader>
              <CardTitle className="flex items-center justify-between">
                <span>{group}</span>
                <span className="text-xs font-normal text-muted-foreground">
                  {routes.length} route{routes.length === 1 ? '' : 's'}
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
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
            No routes match “{filter}”.
          </p>
        )}
      </div>
    </div>
  )
}

function ApiExplorerPending() {
  return (
    <div className="space-y-6">
      <PageHeader title="API explorer" />
      <ListSkeleton rows={8} />
    </div>
  )
}
