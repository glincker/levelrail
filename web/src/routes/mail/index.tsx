import { createFileRoute } from '@tanstack/react-router'
import { requireExperimental } from '../../lib/experimental'
import { MailPage } from '../../components/mailserver/MailPage'
import {
  parseMailTab,
  type MailTab,
} from '../../components/mailserver/mailTabs'

interface MailSearch {
  tab?: MailTab
  add?: boolean
}

export const Route = createFileRoute('/mail/')({
  validateSearch: (s: Record<string, unknown>): MailSearch => ({
    tab: parseMailTab(s.tab),
    add:
      s.add === true || s.add === 1 || s.add === 'true' || s.add === '1'
        ? true
        : undefined,
  }),
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'mail', location.href),
  component: MailRoute,
})

// Deep link: ?tab= picks the section; without it the last tab this viewer used wins.
function MailRoute() {
  const { tab, add } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <MailPage
      tab={tab}
      add={add}
      onTab={(next) =>
        void navigate({ search: (p) => ({ ...p, tab: next }), replace: true })
      }
    />
  )
}
