import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { DomainEditor } from '../../../components/DomainEditor'

// Former "domains" tab, now a real deep-linkable route. Reads app data
// from the query cache the parent layout route's loader already primed.
export const Route = createFileRoute('/apps/$name/domains')({
  component: DomainsSection,
})

function DomainsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return <DomainEditor app={app} />
}
