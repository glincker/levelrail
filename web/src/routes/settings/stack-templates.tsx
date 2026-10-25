import { createFileRoute } from '@tanstack/react-router'
import { requireExperimental } from '../../lib/experimental'
import { StackTemplatesPage } from '../../components/stacks/templates/StackTemplatesPage'

// Experimental (APP_EXPERIMENTAL=iac), same gate as the stacks page.
export const Route = createFileRoute('/settings/stack-templates')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'iac', location.href),
  component: StackTemplatesPage,
})
