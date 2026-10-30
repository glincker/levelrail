import { lazy, Suspense } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { signInRequestsQueryOptions } from '../../queries/signIn'

// The card code stays out of the first load: it only renders while a request is pending.
const SignInRequestsList = lazy(() =>
  import('./SignInRequestsList').then((m) => ({
    default: m.SignInRequestsList,
  })),
)

// Pages whose content already is the sign-in card, so the banner would repeat it.
const OWN_CARD_PATHS = new Set(['/approve-sign-in', '/security'])

// Shown on every other page while someone is signing in to this account with a
// code or from a new browser. It never fetches a code until asked.
export function SignInRequestsBanner() {
  const { data } = useQuery(signInRequestsQueryOptions())
  const { t } = useTranslation('signIn', { useSuspense: false })
  const path = useRouterState({ select: (s) => s.location.pathname })
  const codes = data?.codes ?? []
  const approvals = data?.approvals ?? []
  if (
    (codes.length === 0 && approvals.length === 0) ||
    OWN_CARD_PATHS.has(path)
  ) {
    return null
  }
  return (
    <div role="alert" className="shrink-0 px-4 pt-3 text-sm">
      <p className="sr-only">{t('requests.bannerTitle')}</p>
      <Suspense fallback={null}>
        <SignInRequestsList codes={codes} approvals={approvals} compact />
      </Suspense>
    </div>
  )
}
