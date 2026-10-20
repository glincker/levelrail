import { useEffect, useState } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { isServerReturnPath, safeReturnPath } from '../lib/connectionState'
import { getStoredUsername } from '../lib/authStore'
import { brandQueryOptions } from '../queries/brand'
import { LoginScreen } from '../components/LoginScreen'
import { setupTokenFromHash } from '../lib/setupTokenHash'

interface LoginSearch {
  redirect?: string
  session_link?: string
  approval?: string
  signed_out?: 'session'
}

// Plain function rather than zod so validateSearch stays out of the eagerly loaded bundle.
function validateLoginSearch(search: Record<string, unknown>): LoginSearch {
  const { session_link: sessionLink, approval } = search
  const redirectTo = safeReturnPath(search.redirect)
  return {
    ...(redirectTo ? { redirect: redirectTo } : {}),
    ...(typeof sessionLink === 'string' && sessionLink !== ''
      ? { session_link: sessionLink }
      : {}),
    ...(typeof approval === 'string' && /^la_[\w-]{1,64}$/.test(approval)
      ? { approval }
      : {}),
    ...(search.signed_out === 'session'
      ? { signed_out: 'session' as const }
      : {}),
  }
}

export const Route = createFileRoute('/login')({
  validateSearch: validateLoginSearch,
  beforeLoad: ({ search }) => {
    // A stale stored name must not skip the form on the way back to an app.
    if (search.redirect && isServerReturnPath(search.redirect)) {
      return
    }
    if (getStoredUsername() !== null) {
      redirect({ to: '/', throw: true })
    }
  },
  // Branding is needed before a session exists and this route may be the first to load.
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: LoginPage,
})

function LoginPage() {
  const [setup] = useState(() => setupTokenFromHash(window.location.hash))
  useEffect(() => {
    if (setup) {
      window.history.replaceState(
        null,
        '',
        window.location.pathname + window.location.search,
      )
    }
  }, [setup])
  const {
    session_link: sessionLink,
    approval,
    signed_out: signedOut,
  } = Route.useSearch()
  return (
    <LoginScreen
      setup={setup}
      sessionLink={sessionLink}
      resumeApprovalId={approval}
      sessionEnded={signedOut === 'session'}
    />
  )
}
