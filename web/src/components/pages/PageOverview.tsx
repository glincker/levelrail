import { RecentDeploys } from './RecentDeploys'
import { SiteChecksCard } from './SiteChecksCard'
import { SitePreview } from './SitePreview'
import { SiteTrafficCard } from './SiteTrafficCard'

export default function PageOverview({
  name,
  onAddRedirect,
  onViewDeploys,
}: Readonly<{
  name: string
  onAddRedirect: (path: string) => void
  onViewDeploys: () => void
}>) {
  return (
    <div className="grid gap-3 lg:grid-cols-[22rem_1fr]">
      <SitePreview name={name} />
      <SiteChecksCard name={name} />
      <div className="lg:col-span-2">
        <SiteTrafficCard name={name} onAddRedirect={onAddRedirect} />
      </div>
      <div className="lg:col-span-2">
        <RecentDeploys name={name} onViewAll={onViewDeploys} />
      </div>
    </div>
  )
}
