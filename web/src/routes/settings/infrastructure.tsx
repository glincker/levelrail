import { createFileRoute } from '@tanstack/react-router'
import { requireExperimental } from '../../lib/experimental'
import { InfrastructurePage } from '../../components/settings/InfrastructurePage'

export const Route = createFileRoute('/settings/infrastructure')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'iac', location.href),
  component: InfrastructurePage,
})
