import { GlobeIcon, LayoutIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { useStaticSites } from '../queries/staticSites'

// Read-only summary of build.type: static sites on the
// Apps list page. Static sites are a genuinely different resource kind
// from an app (no container, no store.DesiredService row: see
// internal/store/static_site.go), so they don't belong mixed into
// AppRow's list, but until now a static site deployed via git push had
// no dashboard visibility anywhere at all. Deliberately just a name +
// domains summary: no detail page, no creation flow, and no delete
// action, because none of those exist on the backend yet either
// (GET /api/v1/static-sites is the only route). Renders nothing when
// there are no static sites, so an operator who has never used
// build.type: static never sees an empty card cluttering the page they
// already land on for apps.
export function StaticSitesCard() {
  const { data: sites } = useStaticSites()

  if (sites.length === 0) {
    return null
  }

  return (
    <div className="mt-6 rounded-lg border border-border bg-card">
      <div className="flex items-center gap-2 border-b border-border px-4 py-2.5">
        <LayoutIcon
          className="size-4 text-muted-foreground"
          aria-hidden="true"
        />
        <h2 className="text-sm font-medium text-foreground">Static sites</h2>
        <span className="text-xs text-muted-foreground">
          {sites.length} {sites.length === 1 ? 'site' : 'sites'}
        </span>
      </div>
      <ul>
        {sites.map((site) => (
          <li
            key={site.name}
            className="flex items-center justify-between gap-3 border-b border-border px-4 py-2.5 last:border-b-0"
          >
            <span className="truncate text-sm font-medium text-foreground">
              {site.name}
            </span>
            <span className="flex min-w-0 flex-wrap items-center justify-end gap-1.5">
              {site.domains.length > 0 ? (
                site.domains.map((domain) => (
                  <Badge
                    key={domain}
                    variant="outline"
                    className="gap-1 font-mono text-[11px] text-muted-foreground"
                  >
                    <GlobeIcon className="size-3" aria-hidden="true" />
                    {domain}
                  </Badge>
                ))
              ) : (
                <span className="text-xs text-muted-foreground/60 italic">
                  no domain
                </span>
              )}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
