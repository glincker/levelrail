import { RecentDeploys } from './RecentDeploys'
import { SiteChecksCard } from './SiteChecksCard'
import { SitePreview } from './SitePreview'
import { SiteRecentPreviews } from './SiteRecentPreviews'
import { SiteSummaryStrip } from './SiteSummaryStrip'
import { SiteTrafficCard } from './SiteTrafficCard'
import { SiteWiring } from './SiteWiring'
import type { PageTab } from './pageTabs'

export default function PageOverview({
  name,
  onAddRedirect,
  onOpenTab,
}: Readonly<{
  name: string
  onAddRedirect: (path: string) => void
  onOpenTab: (tab: PageTab) => void
}>) {
  return (
    <div className="space-y-3">
      <SiteSummaryStrip name={name} />
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-3">
          <SiteWiring name={name} onOpenTab={onOpenTab} />
          <RecentDeploys
            name={name}
            onViewAll={() => onOpenTab('deployments')}
          />
          <SiteTrafficCard name={name} onAddRedirect={onAddRedirect} />
        </div>
        <div className="min-w-0 space-y-3">
          <SitePreview name={name} />
          <SiteChecksCard name={name} />
          <SiteRecentPreviews name={name} />
        </div>
      </div>
    </div>
  )
}
