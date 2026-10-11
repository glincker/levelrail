import { createFileRoute, redirect } from '@tanstack/react-router'
import { safeReturnPath } from '../lib/connectionState'
import { getStoredUsername } from '../lib/authStore'
import { brandQueryOptions } from '../queries/brand'
import { LoginScreen } from '../components/LoginScreen'

interface LoginSearch {
  setup?: string
  redirect?: string
  session_link?: string
  approval?: string
}

// Plain function rather than zod so validateSearch stays out of the eagerly loaded bundle.
function validateLoginSearch(search: Record<string, unknown>): LoginSearch {
  const { setup, session_link: sessionLink, approval } = search
  const redirectTo = safeReturnPath(search.redirect)
  return {
    ...(typeof setup === 'string' && setup !== '' ? { setup } : {}),
    ...(redirectTo ? { redirect: redirectTo } : {}),
    ...(typeof sessionLink === 'string' && sessionLink !== ''
      ? { session_link: sessionLink }
      : {}),
    ...(typeof approval === 'string' && /^la_[\w-]{1,64}$/.test(approval)
      ? { approval }
      : {}),
  }
}

export const Route = createFileRoute('/login')({
  validateSearch: validateLoginSearch,
  beforeLoad: () => {
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
  const { setup, session_link: sessionLink, approval } = Route.useSearch()
  return (
    <LoginScreen
      setup={setup}
      sessionLink={sessionLink}
      resumeApprovalId={approval}
    />
  )
}
