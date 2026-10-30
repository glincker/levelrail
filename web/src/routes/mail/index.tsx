import { createFileRoute } from '@tanstack/react-router'
import { requireExperimental } from '../../lib/experimental'
import { MailPage } from '../../components/mailserver/MailPage'

export const Route = createFileRoute('/mail/')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'mail', location.href),
  component: MailPage,
})
