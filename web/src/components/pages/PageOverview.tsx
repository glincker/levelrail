import { SiteChecksCard } from './SiteChecksCard'
import { SiteTrafficCard } from './SiteTrafficCard'

export default function PageOverview({
  name,
  onAddRedirect,
}: Readonly<{ name: string; onAddRedirect: (path: string) => void }>) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <SiteChecksCard name={name} />
      <SiteTrafficCard name={name} onAddRedirect={onAddRedirect} />
    </div>
  )
}
