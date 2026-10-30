import { createFileRoute } from '@tanstack/react-router'
import { dashboardUrlQueryOptions } from '../../../queries/dashboardUrl'
import { ingressSettingsQueryOptions } from '../../../queries/domains'
import { PlatformOverviewPage } from '../../../components/platform/PlatformOverviewPage'

// Loader failures must not block a status board; tiles load on their own.
export const Route = createFileRoute('/settings/platform/')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.prefetchQuery(ingressSettingsQueryOptions()),
      queryClient.prefetchQuery(dashboardUrlQueryOptions()),
    ]),
  component: PlatformOverviewPage,
})
