import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ShieldWarningIcon } from '@phosphor-icons/react/dist/ssr'
import { SignInRequestsList } from '../attention/SignInRequestsBanner'
import { signInRequestsQueryOptions } from '../../queries/signIn'

const POLL_MS = 3000

// Polls fast so a sign-in someone just started shows up without a reload.
// The live region stays mounted so screen readers announce the arrival.
export function SignInApprovalCard({
  showEmpty = false,
  focusOnArrival = false,
}: {
  showEmpty?: boolean
  focusOnArrival?: boolean
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const { data } = useQuery({
    ...signInRequestsQueryOptions(),
    refetchInterval: POLL_MS,
  })
  const approvals = data?.approvals ?? []
  const count = approvals.length
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (focusOnArrival && count > 0) {
      ref.current?.focus()
    }
  }, [focusOnArrival, count])
  return (
    <div aria-live="polite" aria-label={t('approvalCard.regionLabel')}>
      {count > 0 ? (
        <section
          ref={ref}
          tabIndex={-1}
          aria-labelledby="sign-in-approval-title"
          className="space-y-3 rounded-lg border border-primary/40 bg-primary/10 p-4 outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <div className="flex items-center gap-2">
            <ShieldWarningIcon aria-hidden="true" className="size-5 shrink-0" />
            <h2 id="sign-in-approval-title" className="font-semibold">
              {t('approvalCard.title')}
            </h2>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('approvalCard.lead')}
          </p>
          <SignInRequestsList codes={[]} approvals={approvals} />
        </section>
      ) : showEmpty ? (
        <p className="text-sm text-muted-foreground">
          {t('approvalCard.empty')}
        </p>
      ) : null}
    </div>
  )
}
