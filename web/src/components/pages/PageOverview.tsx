import { SiteChecksCard } from './SiteChecksCard'
import { SitePreview } from './SitePreview'
import { SiteTrafficCard } from './SiteTrafficCard'

export default function PageOverview({
  name,
  onAddRedirect,
}: Readonly<{ name: string; onAddRedirect: (path: string) => void }>) {
  return (
    <div className="grid gap-3 lg:grid-cols-[22rem_1fr]">
      <SitePreview name={name} />
      <SiteChecksCard name={name} />
      <div className="lg:col-span-2">
        <SiteTrafficCard name={name} onAddRedirect={onAddRedirect} />
      </div>
    </div>
  )
}
