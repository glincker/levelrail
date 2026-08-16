import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { certificatesQueryOptions } from '../../../queries/certificates'
import { DomainEditor } from '../../../components/DomainEditor'

// Former "domains" tab, now a real deep-linkable route. Reads app data
// from the query cache the parent layout route's loader already primed;
// primes certificates here for DomainEditor's inline TLS status.
export const Route = createFileRoute('/apps/$name/domains')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(certificatesQueryOptions()),
  component: DomainsSection,
})

function DomainsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return <DomainEditor app={app} />
}
