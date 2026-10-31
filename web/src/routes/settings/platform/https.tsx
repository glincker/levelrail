import { createFileRoute } from '@tanstack/react-router'
import { certificatesQueryOptions } from '../../../queries/certificates'
import { PlatformHttpsPage } from '../../../components/platform/PlatformHttpsPage'

export const Route = createFileRoute('/settings/platform/https')({
  loader: ({ context: { queryClient } }) =>
    queryClient.prefetchQuery(certificatesQueryOptions()),
  component: PlatformHttpsPage,
})
