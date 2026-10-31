import { createFileRoute } from '@tanstack/react-router'
import { dashboardUrlQueryOptions } from '../../../queries/dashboardUrl'
import { ingressSettingsQueryOptions } from '../../../queries/domains'
import { PlatformLayout } from '../../../components/platform/PlatformLayout'

// Loader failures must not block the tabs; each body loads on its own.
export const Route = createFileRoute('/settings/platform')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.prefetchQuery(ingressSettingsQueryOptions()),
      queryClient.prefetchQuery(dashboardUrlQueryOptions()),
    ]),
  component: PlatformLayout,
})
