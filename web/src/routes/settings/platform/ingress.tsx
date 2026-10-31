import { createFileRoute } from '@tanstack/react-router'
import { certificatesQueryOptions } from '../../../queries/certificates'
import { ingressSettingsQueryOptions } from '../../../queries/domains'
import { PlatformIngressPage } from '../../../components/platform/PlatformIngressPage'

export const Route = createFileRoute('/settings/platform/ingress')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(ingressSettingsQueryOptions()),
      queryClient.ensureQueryData(certificatesQueryOptions()),
    ]),
  component: PlatformIngressPage,
})
