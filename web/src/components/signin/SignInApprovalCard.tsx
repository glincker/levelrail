import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
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
  const [shown, setShown] = useState(false)
  if (count > 0 && !shown) {
    setShown(true)
  }
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (focusOnArrival && count > 0) {
      ref.current?.focus()
    }
  }, [focusOnArrival, count])
  return (
    <div aria-live="polite" aria-label={t('approvalCard.regionLabel')}>
      {count > 0 || shown ? (
        <section
          ref={ref}
          tabIndex={-1}
          aria-label={t('approvalCard.title')}
          className="outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
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
