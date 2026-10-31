import { createFileRoute } from '@tanstack/react-router'
import { requireExperimental } from '../../lib/experimental'
import { MailPage } from '../../components/mailserver/MailPage'
import {
  parseMailTab,
  type MailTab,
} from '../../components/mailserver/mailTabs'

const flag = (v: unknown): true | undefined =>
  v === true || v === 1 || v === 'true' || v === '1' ? true : undefined

interface MailSearch {
  tab?: MailTab
  add?: boolean
  queue?: boolean
}

export const Route = createFileRoute('/mail/')({
  validateSearch: (s: Record<string, unknown>): MailSearch => ({
    tab: parseMailTab(s.tab),
    add: flag(s.add),
    queue: flag(s.queue),
  }),
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'mail', location.href),
  component: MailRoute,
})

// Deep link: ?tab= picks the section; without it the last tab this viewer used wins.
function MailRoute() {
  const { tab, add, queue } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <MailPage
      tab={tab}
      add={add}
      queue={queue}
      onHandled={() =>
        void navigate({
          search: (p) => ({ ...p, add: undefined, queue: undefined }),
          replace: true,
        })
      }
      onTab={(next) =>
        void navigate({ search: (p) => ({ ...p, tab: next }), replace: true })
      }
    />
  )
}
