import { createFileRoute } from '@tanstack/react-router'
import { dashboardUrlQueryOptions } from '../../../queries/dashboardUrl'
import { PlatformAddressesPage } from '../../../components/platform/PlatformAddressesPage'

export const Route = createFileRoute('/settings/platform/addresses')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(dashboardUrlQueryOptions()),
  component: PlatformAddressesPage,
})
