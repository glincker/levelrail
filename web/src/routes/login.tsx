import { createFileRoute, redirect } from '@tanstack/react-router'
import { safeReturnPath } from '../lib/connectionState'
import { getStoredUsername } from '../lib/authStore'
import { brandQueryOptions } from '../queries/brand'
import { LoginScreen } from '../components/LoginScreen'

interface LoginSearch {
  setup?: string
  redirect?: string
}

// Plain function rather than zod so validateSearch stays out of the eagerly loaded bundle.
function validateLoginSearch(search: Record<string, unknown>): LoginSearch {
  const { setup } = search
  const redirectTo = safeReturnPath(search.redirect)
  return {
    ...(typeof setup === 'string' && setup !== '' ? { setup } : {}),
    ...(redirectTo ? { redirect: redirectTo } : {}),
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
  const { setup } = Route.useSearch()
  return <LoginScreen setup={setup} />
}
