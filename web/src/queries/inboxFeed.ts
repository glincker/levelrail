import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { buildInboxEntries, type InboxEntry } from '../lib/inboxEntries'
import { pollUnlessMissing } from '../lib/pollUnlessMissing'
import { attentionFeedQueryOptions } from './attentionFeed'
import { deployApprovalListQueryOptions } from './deployApprovals'
import { failedDeploysQueryOptions } from './failedDeploys'
import { signInRequestsQueryOptions } from './signIn'
import { useInbox, useInboxState } from './inbox'

// Each source is optional: a forbidden or failing endpoint drops only its own rows.
export function useInboxEntries(archived = false): {
  entries: InboxEntry[]
  isPending: boolean
  hasMore: boolean
  loadMore: () => void
} {
  const { t } = useTranslation('inbox', { useSuspense: false })
  const opts = { retry: false, refetchInterval: pollUnlessMissing(30_000) }
  const alerts = useInbox(false, archived)
  const state = useInboxState()
  const failed = useQuery({ ...failedDeploysQueryOptions(), ...opts })
  const approvals = useQuery({
    ...deployApprovalListQueryOptions('pending'),
    ...opts,
  })
  const signIns = useQuery({ ...signInRequestsQueryOptions(), ...opts })
  const feed = useQuery({ ...attentionFeedQueryOptions(), ...opts })
  const entries = buildInboxEntries({
    alerts: alerts.data?.pages.flat(),
    failedDeploys: failed.data,
    approvals: approvals.data,
    signIns: signIns.data?.approvals,
    feed: feed.data,
    state: state.data,
    t: (key, o) => t(key, { ...o, defaultValue: key }),
  })
  return {
    entries,
    isPending: alerts.isPending && state.isPending,
    hasMore: alerts.hasNextPage,
    loadMore: () => {
      if (!alerts.isFetchingNextPage) void alerts.fetchNextPage()
    },
  }
}
